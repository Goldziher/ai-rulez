package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type SBOMCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestSBOMCLISuite(t *testing.T) {
	suite.Run(t, new(SBOMCLITestSuite))
}

func (s *SBOMCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	testutil.SetupConfigWithMCPServers(s.T(), s.workingDir)
}

func (s *SBOMCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *SBOMCLITestSuite) TestPrintsCycloneDXToStdout() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "sbom", "--format", "cyclonedx")

	var doc map[string]any
	s.Require().NoError(json.Unmarshal([]byte(result.Stdout), &doc))
	s.Equal("CycloneDX", doc["bomFormat"])
	s.Equal("1.6", doc["specVersion"])
	s.NotContains(doc, "hashes")
	s.NotContains(result.Stdout, "timestamp")
}

func (s *SBOMCLITestSuite) TestOutputIsIdenticalAcrossRunsAndWritesFiles() {
	first := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "sbom")
	out := filepath.Join(s.workingDir, "sbom.cdx.json")
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "sbom", "-o", out)

	written, err := os.ReadFile(out)
	s.Require().NoError(err)
	s.Equal(first.Stdout, string(written))
}

func (s *SBOMCLITestSuite) TestRejectsUnknownFormat() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "sbom", "--format", "xml")

	result.AssertOutputContains(s.T(), "unknown --format")
}

func (s *SBOMCLITestSuite) TestSPDXFormat() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "sbom", "--format", "spdx-json")

	s.Contains(result.Stdout, `"spdxVersion": "SPDX-2.3"`)
}
