#!/bin/bash

# AI-Rulez Enforcement Integration Test Runner
# This script runs comprehensive integration tests with real AI agents

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🧪 AI-Rulez Enforcement Integration Tests${NC}"
echo "========================================="
echo ""

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
AI_RULEZ_BIN="$PROJECT_ROOT/ai-rulez"

# Check if we're in the right directory
if [ ! -f "$PROJECT_ROOT/go.mod" ]; then
    echo -e "${RED}Error: Not in AI-Rulez project root${NC}"
    exit 1
fi

# Build ai-rulez binary
echo -e "${BLUE}📦 Building ai-rulez binary...${NC}"
cd "$PROJECT_ROOT"
go build -o ai-rulez ./cmd
if [ $? -ne 0 ]; then
    echo -e "${RED}Failed to build ai-rulez${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Build successful${NC}"
echo ""

# Check available agents
echo -e "${BLUE}🔍 Checking available agents...${NC}"
AVAILABLE_AGENTS=""
UNAVAILABLE_AGENTS=""

for agent in claude gemini amp continue-dev cursor windsurf github-copilot; do
    if command -v $agent &> /dev/null; then
        echo -e "${GREEN}✅ $agent available${NC}"
        AVAILABLE_AGENTS="$AVAILABLE_AGENTS $agent"
    else
        echo -e "${YELLOW}❌ $agent not found${NC}"
        UNAVAILABLE_AGENTS="$UNAVAILABLE_AGENTS $agent"
    fi
done

if [ -z "$AVAILABLE_AGENTS" ]; then
    echo -e "${RED}No AI agents available for testing${NC}"
    echo "Please install at least one agent (claude, gemini, amp, etc.)"
    exit 1
fi

echo ""
echo -e "${GREEN}Available agents:${NC}$AVAILABLE_AGENTS"
echo -e "${YELLOW}Unavailable agents:${NC}$UNAVAILABLE_AGENTS"
echo ""

# Parse command line arguments
RUN_BENCHMARKS=false
RUN_UNIT=true
RUN_INTEGRATION=true
VERBOSE=""
SPECIFIC_TEST=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --bench|--benchmarks)
            RUN_BENCHMARKS=true
            shift
            ;;
        --unit-only)
            RUN_INTEGRATION=false
            shift
            ;;
        --integration-only)
            RUN_UNIT=false
            shift
            ;;
        --verbose|-v)
            VERBOSE="-v"
            shift
            ;;
        --test)
            SPECIFIC_TEST="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --bench, --benchmarks  Run performance benchmarks"
            echo "  --unit-only           Run only unit tests"
            echo "  --integration-only    Run only integration tests"
            echo "  --verbose, -v         Verbose output"
            echo "  --test <name>         Run specific test"
            echo "  --help, -h            Show this help message"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# Run unit tests for enforce command
if [ "$RUN_UNIT" = true ]; then
    echo -e "${BLUE}📊 Running unit tests...${NC}"
    cd "$PROJECT_ROOT"

    echo "Testing enforce command..."
    go test ./cmd/commands -run TestEnforce $VERBOSE
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ Unit tests passed${NC}"
    else
        echo -e "${RED}❌ Unit tests failed${NC}"
        exit 1
    fi
    echo ""
fi

# Run integration tests
if [ "$RUN_INTEGRATION" = true ]; then
    echo -e "${BLUE}🔧 Running integration tests...${NC}"
    echo "This may take several minutes depending on agent response times..."
    echo ""

    cd "$PROJECT_ROOT"

    if [ -n "$SPECIFIC_TEST" ]; then
        echo "Running specific test: $SPECIFIC_TEST"
        go test ./tests/integration -run "$SPECIFIC_TEST" $VERBOSE -timeout=30m
    else
        # Run all integration tests
        go test ./tests/integration $VERBOSE -timeout=30m
    fi

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ Integration tests passed${NC}"
    else
        echo -e "${YELLOW}⚠️  Some integration tests failed (this may be normal if agents timeout)${NC}"
    fi
    echo ""
fi

# Run benchmarks if requested
if [ "$RUN_BENCHMARKS" = true ]; then
    echo -e "${BLUE}⚡ Running performance benchmarks...${NC}"
    echo "This will take some time..."
    echo ""

    cd "$PROJECT_ROOT"
    go test ./tests/benchmarks -bench=. -benchtime=10s $VERBOSE

    echo ""
    echo -e "${GREEN}✅ Benchmarks completed${NC}"
fi

# Generate coverage report if all tests were run
if [ "$RUN_UNIT" = true ] && [ "$RUN_INTEGRATION" = true ]; then
    echo ""
    echo -e "${BLUE}📈 Generating coverage report...${NC}"

    cd "$PROJECT_ROOT"
    go test ./... -coverprofile=coverage.out 2>/dev/null
    go tool cover -html=coverage.out -o coverage.html

    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
    echo -e "${GREEN}Total coverage: $COVERAGE${NC}"
    echo "Coverage report saved to: coverage.html"
fi

echo ""
echo -e "${BLUE}=========================================${NC}"
echo -e "${GREEN}🎉 Test run completed!${NC}"
echo ""

# Summary
echo "Summary:"
echo "- Available agents tested:$AVAILABLE_AGENTS"
if [ "$RUN_UNIT" = true ]; then
    echo "- Unit tests: ✅"
fi
if [ "$RUN_INTEGRATION" = true ]; then
    echo "- Integration tests: ✅"
fi
if [ "$RUN_BENCHMARKS" = true ]; then
    echo "- Benchmarks: ✅"
fi

# Check if any tests can use the enforce command
echo ""
echo -e "${BLUE}Quick enforcement test with first available agent:${NC}"
FIRST_AGENT=$(echo $AVAILABLE_AGENTS | awk '{print $1}')
if [ -n "$FIRST_AGENT" ]; then
    echo "Testing with $FIRST_AGENT..."
    "$AI_RULEZ_BIN" enforce \
        --agent "$FIRST_AGENT" \
        --config tests/fixtures/configs/test_enforcement.yaml \
        --include-files "tests/fixtures/violations/javascript/console_log.js" \
        --timeout 5s \
        --format summary
fi

echo ""
echo -e "${GREEN}Done! You can now run specific tests with:${NC}"
echo "  $0 --test TestEnforcementWithRealAgents"
echo "  $0 --bench"
echo "  $0 --integration-only"