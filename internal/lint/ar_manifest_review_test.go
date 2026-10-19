package lint

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTyposquatRealPackagesAreNotReported(t *testing.T) {
	for _, tc := range []struct {
		file string
		u    nameUniverse
	}{
		{"testdata/ar056_real_pypi.txt", pyPIUniverse()},
		{"testdata/ar056_real_npm.txt", npmUniverse()},
	} {
		data, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		names := strings.Fields(string(data))
		outside := 0
		for _, n := range names {
			if !tc.u.real[n] {
				outside++
			}
			if near := tc.u.typosquatOf(n); near != "" {
				t.Errorf("%s: real package %q reported as a squat of %q", tc.file, n, near)
			}
		}
		if len(names) < 400 || outside < 30 {
			t.Errorf("%s: the sample (%d names, %d outside the rankings) is too small to prove anything", tc.file, len(names), outside)
		}
	}
	for _, n := range []string{"ijson", "scrapyd", "cattrs", "aiofile", "pyaml", "usort"} {
		if near := pyPIUniverse().typosquatOf(n); near != "" {
			t.Errorf("%s reported as a squat of %s", n, near)
		}
	}
}

func TestManifestLinesAndParsersAR054(t *testing.T) {
	body := skillDoc("", "x\n")
	proj := func(content string) map[string]string { return manifestFiles("pyproject.toml", content) }
	req := func(name, content string) map[string]string { return manifestFiles(name, content) }
	pkg := func(content string) map[string]string { return manifestFiles("package.json", content) }
	warn := map[string]Severity{"AR054": SeverityWarning}
	runRuleCases(t, []ruleCase{
		{name: "pyproject line is the dependency, not the project name", skill: body, files: proj("[project]\nname = \"requests-helper\"\ndependencies = [\n  \"requests\",\n]\n"), want: []string{"AR054:pyproject.toml:4"}},
		{name: "pyproject same name as the project", skill: body, files: proj("[project]\nname = \"requests\"\ndependencies = [\"requests\"]\n"), want: []string{"AR054:pyproject.toml:3"}},
		{name: "poetry dependencies", skill: body, files: proj("[tool.poetry.dependencies]\npython = \"^3.11\"\nrequests = \"*\"\nclick = \"8.1.7\"\nrich = {version = \"^13\"}\nlib = {git = \"https://github.com/o/lib.git\"}\n"), want: []string{"AR054:pyproject.toml:3", "AR054:pyproject.toml:5", "AR054:pyproject.toml:6"}},
		{name: "poetry exact pins and pinned git", skill: body, files: proj("[tool.poetry.dependencies]\npython = \"^3.11\"\nclick = \"8.1.7\"\nlib = {git = \"https://github.com/o/lib.git\", rev = \"0123456789abcdef0123456789abcdef01234567\"}\nlocal = {path = \"../local\"}\n"), absent: []string{"AR054"}},
		{name: "poetry group", skill: body, files: proj("[tool.poetry.group.dev.dependencies]\npytest = \">=7\"\n"), want: []string{"AR054:pyproject.toml:2"}, sev: warn},
		{name: "pdm dev dependencies", skill: body, files: proj("[tool.pdm.dev-dependencies]\ntest = [\"pytest\"]\n"), want: []string{"AR054:pyproject.toml:2"}},
		{name: "build-system requires", skill: body, files: proj("[build-system]\nrequires = [\"setuptools>=61\"]\nbuild-backend = \"setuptools.build_meta\"\n"), want: []string{"AR054:pyproject.toml:2"}},
		{name: "a wrongly typed uv key does not hide the dependencies", skill: body, files: proj("[project]\ndependencies = [\"requests\"]\n[tool.uv]\nindex = \"x\"\n"), want: []string{"AR054:pyproject.toml:2"}},
		{name: "package.json with a non-string value", skill: body, files: pkg("{\n  \"dependencies\": {\n    \"a\": \"*\",\n    \"b\": 1\n  }\n}\n"), want: []string{"AR054:package.json:3"}},
		{name: "dev-requirements.txt", skill: body, files: req("dev-requirements.txt", "pytest\n"), want: []string{"AR054:dev-requirements.txt:1"}},
		{name: "requirements directory", skill: body, files: req("requirements/base.txt", "pytest\n"), want: []string{"AR054:base.txt:1"}},
		{name: "requirements.lock", skill: body, files: req("requirements.lock", "pytest\n"), want: []string{"AR054:requirements.lock:1"}},
		{name: "byte order mark on the first line", skill: body, files: req("requirements.txt", "\ufeffrequests\nclick==8.1.7\n"), want: []string{"AR054:requirements.txt:1"}},
		{name: "byte order mark in package.json", skill: body, files: pkg("\ufeff{\"dependencies\": {\"left-pad\": \"latest\"}}"), want: []string{"AR054:package.json:0"}},
		{name: "invalid byte after a valid line", skill: body, files: req("requirements.txt", "requests\n\xff\xfe\nclick==8.1.7\n"), want: []string{"AR054:requirements.txt:1"}},
		{name: "utf-16 requirements", skill: body, files: req("requirements.txt", utf16Text("requests\r\nclick==8.1.7\r\n")), want: []string{"AR054:requirements.txt:1"}},
		{name: "binary manifest is not analyzable", skill: body, files: req("requirements.txt", "abc\x00def\x00"), want: []string{"AR054:requirements.txt:1"}, sev: map[string]Severity{"AR054": SeverityInfo}},
		{name: "oversized manifest is not analyzable", skill: body, files: req("requirements.txt", strings.Repeat("# padding padding padding padding\n", 20000)+"requests\n"), want: []string{"AR054:requirements.txt:1"}, sev: map[string]Severity{"AR054": SeverityInfo}},
		{name: "skill root requirements", skill: body, files: map[string]string{".ai-rulez/skills/bad/requirements.txt": "requests\n"}, want: []string{"AR054:requirements.txt:1"}},
		{name: "skill root package.json", skill: body, files: map[string]string{".ai-rulez/skills/bad/package.json": "{\"dependencies\": {\"x\": \"latest\"}}"}, want: []string{"AR054:package.json:0"}},
		{name: "manifest in a lib directory", skill: body, files: map[string]string{".ai-rulez/skills/bad/lib/requirements.txt": "requests\n"}, want: []string{"AR054:requirements.txt:1"}},
	})
}

func utf16Text(s string) string {
	units := utf16.Encode([]rune("\ufeff" + s))
	b := make([]byte, 0, 2*len(units))
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	return string(b)
}

func TestSourceRedirectParsersAR055(t *testing.T) {
	body := skillDoc("", "x\n")
	files := func(name, content string) map[string]string {
		return map[string]string{".ai-rulez/skills/bad/" + name: content}
	}
	const evil = "https://pkgs.evil.example/simple"
	runRuleCases(t, []ruleCase{
		{name: "skill root npmrc", skill: body, files: files(".npmrc", "registry=https://npm.evil.example/\n"), want: []string{"AR055:.npmrc:1"}},
		{name: "skill root pip.conf", skill: body, files: files("pip.conf", "[global]\nindex-url = "+evil+"\n"), want: []string{"AR055:pip.conf:2"}},
		{name: "yarnrc.yml registry", skill: body, files: files(".yarnrc.yml", "npmRegistryServer: \"https://npm.evil.example/\"\n"), want: []string{"AR055:.yarnrc.yml:1"}},
		{name: "yarnrc.yml scoped registry is a dependency-confusion defense", skill: body, files: files(".yarnrc.yml", "npmScopes:\n  acme:\n    npmRegistryServer: https://npm.evil.example/\n"), absent: []string{"AR055"}},
		{name: "classic yarnrc", skill: body, files: files(".yarnrc", "registry \"https://npm.evil.example/\"\n"), want: []string{"AR055:.yarnrc:1"}},
		{name: "pip.ini mixed case key", skill: body, files: files("pip.ini", "[global]\nIndex-URL = "+evil+"\n"), want: []string{"AR055:pip.ini:2"}},
		{name: "uv default-index flag", skill: body, files: files("requirements.txt", "--default-index "+evil+"\nrequests==2.32.3\n"), want: []string{"AR055:requirements.txt:1"}},
		{name: "uv named index flag", skill: body, files: files("requirements.txt", "--index corp="+evil+"\n"), want: []string{"AR055:requirements.txt:1"}},
		{name: "uv named index to the default host", skill: body, files: files("requirements.txt", "--index pypi=https://pypi.org/simple\n"), absent: []string{"AR055"}},
		{name: "uv.toml index table", skill: body, files: files("uv.toml", "[[index]]\nname = \"corp\"\nurl = \""+evil+"\"\n"), want: []string{"AR055:uv.toml:3"}},
		{name: "uv.toml index-url", skill: body, files: files("uv.toml", "index-url = \""+evil+"\"\n"), want: []string{"AR055:uv.toml:1"}},
		{name: "pdm source", skill: body, files: files("pyproject.toml", "[[tool.pdm.source]]\nname = \"corp\"\nurl = \""+evil+"\"\n"), want: []string{"AR055:pyproject.toml:3"}},
		{name: "poetry source", skill: body, files: files("pyproject.toml", "[[tool.poetry.source]]\nname = \"corp\"\nurl = \""+evil+"\"\n"), want: []string{"AR055:pyproject.toml:3"}},
		{name: "pytorch wheel index is a default host", skill: body, files: files("requirements.txt", "--index-url https://download.pytorch.org/whl/cpu\ntorch==2.4.0\n"), absent: []string{"AR055"}},
		{name: "legacy pypi host", skill: body, files: files("requirements.txt", "--index-url https://pypi.python.org/simple\n"), absent: []string{"AR055"}},
	})
}

func TestManifestCapAndSpeed(t *testing.T) {
	body := skillDoc("", "x\n")
	var proj, pkg strings.Builder
	proj.WriteString("[project]\nname = \"big\"\ndependencies = [\n")
	pkg.WriteString("{\n  \"dependencies\": {\n")
	const n = 20000
	for i := range n {
		fmt.Fprintf(&proj, "  \"dep%05d\",\n", i)
		sep := ","
		if i == n-1 {
			sep = ""
		}
		fmt.Fprintf(&pkg, "    \"lib%05d\": \"*\"%s\n", i, sep)
	}
	proj.WriteString("]\n")
	pkg.WriteString("  }\n}\n")
	for _, tc := range []struct{ name, content string }{{"pyproject.toml", proj.String()}, {"package.json", pkg.String()}} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.content) > maxManifestBytes {
				t.Fatalf("fixture is %d bytes, over the cap", len(tc.content))
			}
			fs := lintFiles(t, map[string]string{".ai-rulez/config.toml": baseConfig, ".ai-rulez/skills/bad/SKILL.md": body, ".ai-rulez/skills/bad/scripts/" + tc.name: tc.content})
			if got := countCode(fs, "AR054"); got != maxManifestFindings+1 {
				t.Errorf("AR054 findings = %d, want %d plus one summary", got, maxManifestFindings)
			}
			found := false
			for _, f := range fs {
				found = found || (f.Code == "AR054" && strings.Contains(f.Message, "more dependencies"))
			}
			if !found {
				t.Error("no summary of the findings the cap left out")
			}
		})
	}
}

func TestDependencyLineIndex(t *testing.T) {
	x := newLineIndex(strings.Split("[project]\nname = \"a\"\ndependencies = [\n  \"a\",\n  \"b\",\n  \"a\",\n]\n[tool.poetry.dependencies]\nrequests = \"*\"\n", "\n"))
	for _, tc := range []struct {
		got, want int
	}{{x.literal("a"), 4}, {x.literal("a"), 6}, {x.literal("a"), 1}, {x.literal("b"), 5}, {x.key("requests"), 9}, {x.key("missing"), 1}} {
		if tc.got != tc.want {
			t.Errorf("line = %d, want %d", tc.got, tc.want)
		}
	}
}

// TestOneEditMatchesLevenshtein keeps the allocation-free check equal to the
// reference definition: one Levenshtein edit, or one adjacent swap.
func TestOneEditMatchesLevenshtein(t *testing.T) {
	words := []string{"", "a", "ab", "ba", "abc", "acb", "abd", "abcd", "bcd", "abcde", "requests", "reqeusts", "request", "requestss", "reqests", "aaa", "aab", "baa"}
	for _, a := range words {
		for _, b := range words {
			want := a != b && editDistance(a, b) == 1
			if !want && len(a) == len(b) && a != b {
				for i := 0; i+1 < len(a); i++ {
					swapped := a[:i] + string(a[i+1]) + string(a[i]) + a[i+2:]
					want = want || swapped == b
				}
			}
			if got := oneEdit(a, b); got != want {
				t.Errorf("oneEdit(%q, %q) = %v, want %v", a, b, got, want)
			}
		}
	}
}
