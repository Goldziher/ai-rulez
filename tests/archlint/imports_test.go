package archlint

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// The library surface must not link the Sigstore signer and KMS stack (issue
// #290): internal/signing holds the verification policy and a Backend seam, and
// only the command wires internal/signing/sigstore in.
const (
	modulePath       = "github.com/Goldziher/ai-rulez/v5"
	heavySigningPath = modulePath + "/internal/signing/sigstore"
)

// heavyModulePrefixes are the module graphs the heavy signing package brings.
var heavyModulePrefixes = []string{
	"github.com/sigstore/",
	"github.com/hashicorp/vault",
	"github.com/aws/aws-sdk-go-v2",
	"github.com/Azure/azure-sdk-for-go",
	"cloud.google.com/go/kms",
}

// libraryRoots are the packages that must stay free of the heavy signing code.
var libraryRoots = []string{
	"./pkg/airulez",
	"./internal/govview",
	"./internal/mcp/...",
	"./internal/policy",
	"./internal/approval",
	"./internal/lockrun",
}

func TestLibraryPackagesDoNotLinkTheHeavySigningStack(t *testing.T) {
	root := repoRoot(t)
	for _, pkg := range libraryRoots {
		t.Run(pkg, func(t *testing.T) {
			for _, dep := range depsOf(t, root, pkg) {
				if dep == heavySigningPath || strings.HasPrefix(dep, heavySigningPath+"/") {
					t.Errorf("%s imports %s: take the verification seam in internal/signing (signing.Backend) and let the command call signing.UseBackend", pkg, dep)
				}
				for _, prefix := range heavyModulePrefixes {
					if strings.HasPrefix(dep, prefix) {
						t.Errorf("%s links %s: the Sigstore and KMS SDKs belong to internal/signing/sigstore", pkg, dep)
					}
				}
			}
		})
	}
}

// TestDepScannerSeesTheHeavyPackage proves depsOf reports the heavy package for
// the command that does import it, so a scan that finds nothing cannot pass.
func TestDepScannerSeesTheHeavyPackage(t *testing.T) {
	deps := depsOf(t, repoRoot(t), "./cmd/ai-rulez")
	found := sort.SearchStrings(deps, heavySigningPath)
	if found == len(deps) || deps[found] != heavySigningPath {
		t.Fatalf("./cmd/ai-rulez does not depend on %s: the command must wire the Sigstore backend", heavySigningPath)
	}
}

// depsOf lists the import paths pkg depends on, sorted.
func depsOf(t *testing.T, root, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	deps := strings.Fields(string(out))
	sort.Strings(deps)
	return deps
}
