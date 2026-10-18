package harnessnames

import (
	"strings"
	"testing"
	"time"
)

func TestEmbeddedTablesAreValid(t *testing.T) {
	// Arrange
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	// Act / Assert
	presets := Presets()
	if len(presets) != 52 {
		t.Fatalf("%d tables, want one per harness preset (52)", len(presets))
	}
	for _, p := range presets {
		tbl, _ := For(p)
		if err := tbl.Validate(p, today); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestEveryRecordedOutcomeHasASourceAndKnownEvidence(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, p := range Presets() {
		tbl, _ := For(p)
		for i, g := range tbl.Group {
			if _, ok := tbl.SourceByID(g.Source); !ok {
				t.Errorf("%s group %d: no source", p, i+1)
			}
		}
	}
}

func TestRolloutPresetsQuoteTheirKeyCollisionOutcome(t *testing.T) {
	// Arrange: the presets that report AR9U0 as an error need a verbatim quote for the
	// outcome of their main group, or the rule would assert what no page was checked for
	want := map[string][]string{
		"claude": {"init", "code-review"}, "gemini": {"help"}, "qwen": {"init"},
		"opencode": {"init"}, "kilo": {"goal"},
	}
	for preset, names := range want {
		tbl, ok := For(preset)
		if !ok {
			t.Fatalf("no table for %s", preset)
		}
		for _, n := range names {
			// Act
			hits := tbl.Find(KindCommand, n)
			// Assert
			if len(hits) == 0 || !hits[0].Quoted {
				t.Errorf("%s %q: no quoted evidence (%+v)", preset, n, hits)
			}
		}
	}
}

func TestFind(t *testing.T) {
	claude, _ := For("claude")
	gemini, _ := For("gemini")
	codex, _ := For("codex")
	tests := []struct {
		name     string
		tbl      *Table
		kind     string
		query    string
		wantName string // recorded name of the first hit, "" for none
		wantColl string
		alias    bool
	}{
		{"exact command", claude, KindSkill, "init", "init", UserWins, false},
		{"casefold and trim", claude, KindSkill, "  INIT ", "init", UserWins, false},
		{"NFKC fullwidth", claude, KindSkill, "ｉｎｉｔ", "init", UserWins, false},
		{"alias", claude, KindSkill, "bg", "bg", UserWins, true},
		{"reserved exact", claude, KindSkill, "synced", "synced", Error, false},
		{"reserved prefix", claude, KindSkill, "anthropic-skills:foo", "anthropic-skills:", Error, false},
		{"reserved does not apply to agents", claude, KindAgent, "synced", "", "", false},
		{"agent only matches agents", claude, KindAgent, "init", "", "", false},
		{"user wins agent", claude, KindAgent, "Explore", "Explore", UserWins, false},
		{"builtin wins", gemini, KindSkill, "help", "help", BuiltinWins, false},
		{"no match", claude, KindSkill, "acme-deploy", "", "", false},
		{"codex slash commands are not a skill namespace", codex, KindSkill, "model", "", "", false},
		{"codex skill both listed", codex, KindSkill, "imagegen", "imagegen", BothListed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			hits := tt.tbl.Find(tt.kind, tt.query)
			// Assert
			if tt.wantName == "" {
				if len(hits) != 0 {
					t.Fatalf("got %+v, want none", hits)
				}
				return
			}
			if len(hits) == 0 {
				t.Fatalf("no hit for %q", tt.query)
			}
			h := hits[0]
			if h.Name != tt.wantName || h.Collision != tt.wantColl || h.Alias != tt.alias {
				t.Errorf("got %+v, want name %q collision %q alias %v", h, tt.wantName, tt.wantColl, tt.alias)
			}
		})
	}
}

func TestFindAliasNamesItsCommand(t *testing.T) {
	claude, _ := For("claude")
	h := claude.Find(KindSkill, "rc")
	if len(h) == 0 || !h[0].Alias || h[0].AliasOf != "remote-control" {
		t.Fatalf("got %+v", h)
	}
}

func TestParseProjectAndMerge(t *testing.T) {
	// Arrange
	claude, _ := For("claude")
	proj, err := ParseProject([]byte(`
schema = 1
preset = "claude"
severity = "error"
[[group]]
kind = "command"
names = ["acme-deploy"]
user_collision = "builtin_wins"
`), "claude")
	if err != nil {
		t.Fatal(err)
	}
	// Act
	merged := claude.WithProject(proj)
	hits := merged.Find(KindSkill, "acme-deploy")
	// Assert
	if len(hits) != 1 || !hits[0].Declared || hits[0].Severity != SeverityError || hits[0].Collision != BuiltinWins {
		t.Fatalf("got %+v", hits)
	}
	if len(claude.Find(KindSkill, "acme-deploy")) != 0 {
		t.Error("the embedded table was modified")
	}
	if len(merged.Find(KindSkill, "init")) == 0 {
		t.Error("the embedded names are gone after the merge")
	}
}

func TestParseProjectRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"wrong preset", "schema = 1\npreset = \"cursor\"\n", "file name"},
		{"unknown key", "schema = 1\npreset = \"claude\"\nbogus = 1\n", "strict mode"},
		{"bad kind", "schema = 1\npreset = \"claude\"\n[[group]]\nkind = \"rule\"\nnames = [\"x\"]\n", "kind"},
		{"no names", "schema = 1\npreset = \"claude\"\n[[group]]\nkind = \"skill\"\n", "no names"},
		{"slash in name", "schema = 1\npreset = \"claude\"\n[[group]]\nkind = \"skill\"\nnames = [\"a/b\"]\n", "valid name"},
		{"bad severity", "schema = 1\npreset = \"claude\"\nseverity = \"info\"\n", "severity"},
		{"bad schema", "schema = 2\npreset = \"claude\"\n", "schema"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseProject([]byte(tt.body), "claude")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDeclared(t *testing.T) {
	d := Declared("claude", []string{"Acme-Deploy"})
	hits := d.Find(KindSkill, "acme-deploy")
	if len(hits) == 0 || hits[0].Collision != Unknown {
		t.Fatalf("got %+v", hits)
	}
	if Declared("claude", nil) != nil {
		t.Error("no names must give no table")
	}
}

func TestValidateRejectsBrokenTables(t *testing.T) {
	good := `
schema = 1
preset = "x"
harness_version = "1"
verified_on = "2026-01-01"
completeness = "partial"
severity = "warning"
[[source]]
id = "s1"
url = "https://example.com"
verified_on = "2026-01-01"
[[group]]
kind = "command"
source = "s1"
user_collision = "unknown"
match = "exact"
names = ["a"]
`
	tests := []struct{ name, from, to, want string }{
		{"valid", "", "", ""},
		{"future date", `verified_on = "2026-01-01"
completeness`, `verified_on = "2999-01-01"
completeness`, "past date"},
		{"missing source", `source = "s1"`, `source = "s9"`, "not declared"},
		{"duplicate name", `names = ["a"]`, `names = ["a", "A"]`, "listed twice"},
		{"bad collision", `"unknown"`, `"maybe"`, "user_collision"},
		{"http source", "https://example.com", "http://example.com", "not https"},
		{"reserved needs error", `kind = "command"`, `kind = "reserved"`, "reserved group"},
		{"none with groups", `"partial"`, `"none"`, "completeness = none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := good
			if tt.from != "" {
				body = strings.Replace(good, tt.from, tt.to, 1)
			}
			tbl, err := Parse([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			err = tbl.Validate("x", "2026-10-18")
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEntries(t *testing.T) {
	claude, _ := For("claude")
	got := claude.Entries([]string{KindCommand, KindSkill}, UserWins)
	set := map[string]Entry{}
	for _, e := range got {
		set[e.Name] = e
	}
	if _, ok := set["init"]; !ok {
		t.Error("init missing")
	}
	if e := set["bg"]; !e.Alias || e.AliasOf != "background" {
		t.Errorf("bg = %+v", e)
	}
	if _, ok := set["synced"]; ok {
		t.Error("a reserved name is not a user_wins entry")
	}
}
