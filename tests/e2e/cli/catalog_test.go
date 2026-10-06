package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type CatalogCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestCatalogCLISuite(t *testing.T) {
	suite.Run(t, new(CatalogCLITestSuite))
}

func (s *CatalogCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	testutil.SetupConfigWithMCPServers(s.T(), s.workingDir)
}

func (s *CatalogCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *CatalogCLITestSuite) TestWritesAReproducibleSite() {
	first := filepath.Join(s.workingDir, "site-a")
	second := filepath.Join(s.workingDir, "site-b")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "catalog", "--html", first)
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "catalog", "--html", second)

	for _, name := range []string{"index.html", "catalog.json", "assets/catalog.css", ".ai-rulez-catalog"} {
		a, err := os.ReadFile(filepath.Join(first, filepath.FromSlash(name)))
		s.Require().NoError(err, name)
		b, err := os.ReadFile(filepath.Join(second, filepath.FromSlash(name)))
		s.Require().NoError(err, name)
		s.Equal(string(a), string(b), name)
	}
}

func (s *CatalogCLITestSuite) TestRefusesAnUnmarkedDirectory() {
	dir := filepath.Join(s.workingDir, "precious")
	s.Require().NoError(os.MkdirAll(dir, 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644))

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "catalog", "--html", dir, "--clean")

	result.AssertOutputContains(s.T(), ".ai-rulez-catalog")
	s.FileExists(filepath.Join(dir, "keep.txt"))
}
