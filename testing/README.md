# AI Rules Testing Data

This directory contains synthetic test data for comprehensive testing of the ai_rules CLI tool.

## Directory Structure

### Scenarios (`scenarios/`)
Test scenarios covering different use cases:

- **`basic/`** - Simple project with inline rules
- **`with-includes/`** - Project using include files and custom templates
- **`nested-includes/`** - Testing deeply nested include resolution
- **`custom-template/`** - Custom template usage examples
- **`empty-project/`** - Project with no rules (edge case)
- **`minimal/`** - Bare minimum valid configuration
- **`invalid/`** - Various invalid configurations for error testing
- **`circular/`** - Circular include dependency (should fail)

### Includes (`includes/`)
Reusable rule files:

- **`react.yaml`** - React-specific coding rules
- **`typescript.yaml`** - TypeScript best practices
- **`security.yaml`** - Security guidelines

### Templates (`templates/`)
Custom template examples:

- **`simple.tmpl`** - Basic text format
- **`detailed.tmpl`** - Rich markdown with priority grouping

## Testing Workflow

1. **Basic Generation:**
   ```bash
   cd testing/scenarios/basic
   ai_rules generate
   ```

2. **Include Resolution:**
   ```bash
   cd testing/scenarios/with-includes
   ai_rules generate
   ```

3. **Validation Testing:**
   ```bash
   cd testing/scenarios/invalid
   ai_rules validate bad-yaml.yaml  # Should fail
   ```

4. **Custom Templates:**
   ```bash
   cd testing/scenarios/custom-template
   ai_rules generate
   ```

## Expected Behavior

- All scenarios in `scenarios/` (except `invalid/` and `circular/`) should generate successfully
- `invalid/` scenarios should fail with appropriate error messages
- `circular/` should detect and report circular dependencies
- Generated files should only be written when content changes (incremental generation)

## File Contents

Each scenario tests specific functionality:
- **Metadata handling** (name, version, description)
- **Include resolution** (relative paths, nested includes)
- **Rule merging** (priority, overrides)
- **Template processing** (default and custom templates)
- **Output generation** (multiple files, directory creation)
- **Error handling** (validation, missing files, circular deps)