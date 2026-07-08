package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// TestAllAgents_IncludeOverridesBuiltin_Deterministic locks the precedence
// contract: a same-named agent from a higher layer (local > include > builtin)
// always wins, regardless of whether the module ships it as a root agent or an
// include-domain, and regardless of that domain's alphabetical position relative
// to the builtin domain. Previously the winner flipped on domain name ordering
// and root-vs-domain placement.
func TestAllAgents_IncludeOverridesBuiltin_Deterministic(t *testing.T) {
	newTree := func(mod func(*config.ContentTree)) *config.ContentTree {
		ct := &config.ContentTree{Domains: map[string]*config.Domain{
			"ai-governance": {
				Name:    "ai-governance",
				Builtin: true,
				Agents:  []config.ContentFile{{Name: "code-reviewer", Content: "GENERIC-BUILTIN"}},
			},
		}}
		mod(ct)
		return ct
	}
	cases := map[string]*config.ContentTree{
		"module as root agent": newTree(func(ct *config.ContentTree) {
			ct.Agents = []config.ContentFile{{Name: "code-reviewer", Content: "RICH"}}
		}),
		"module domain sorts before ai-governance": newTree(func(ct *config.ContentTree) {
			ct.Domains["aaa-shared"] = &config.Domain{
				Name: "aaa-shared", FromInclude: true,
				Agents: []config.ContentFile{{Name: "code-reviewer", Content: "RICH"}},
			}
		}),
		"module domain sorts after ai-governance": newTree(func(ct *config.ContentTree) {
			ct.Domains["core"] = &config.Domain{
				Name: "core", FromInclude: true,
				Agents: []config.ContentFile{{Name: "code-reviewer", Content: "RICH"}},
			}
		}),
	}
	for name, ct := range cases {
		t.Run(name, func(t *testing.T) {
			got := allAgents(ct)
			var n int
			var content string
			for _, a := range got {
				if a.Name == "code-reviewer" {
					n++
					content = a.Content
				}
			}
			if n != 1 {
				t.Fatalf("expected exactly 1 code-reviewer, got %d", n)
			}
			if content != "RICH" {
				t.Errorf("expected include/local body to win, got %q", content)
			}
		})
	}
}

// TestAllAgents_Extends_AppendsToBase verifies the `extends` directive: an agent
// that extends a lower-layer same-named agent inherits its body and frontmatter
// and appends its own message; set frontmatter fields override the base, omitted
// fields inherit, and the `extends` key is dropped from output.
func TestAllAgents_Extends_AppendsToBase(t *testing.T) {
	ct := &config.ContentTree{
		Agents: []config.ContentFile{{
			Name:    "code-reviewer",
			Content: "Project rule: no unwrap in library code.",
			Metadata: &config.Metadata{
				Effort: "high",
				Extra:  map[string]string{"extends": "code-reviewer", "model": "opus"},
			},
		}},
		Domains: map[string]*config.Domain{
			"ai-governance": {
				Name:    "ai-governance",
				Builtin: true,
				Agents: []config.ContentFile{{
					Name:    "code-reviewer",
					Content: "You are a code reviewer.",
					Metadata: &config.Metadata{
						Effort: "medium",
						Tools:  []string{"Read", "Grep", "Glob"},
						Extra:  map[string]string{"model": "sonnet"},
					},
				}},
			},
		},
	}
	got := allAgents(ct)
	var cr *config.ContentFile
	var count int
	for i := range got {
		if got[i].Name == "code-reviewer" {
			count++
			cr = &got[i]
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 code-reviewer, got %d", count)
	}
	wantBody := "You are a code reviewer.\n\nProject rule: no unwrap in library code."
	if cr.Content != wantBody {
		t.Errorf("body = %q, want %q", cr.Content, wantBody)
	}
	if cr.Metadata == nil {
		t.Fatal("expected merged metadata")
	}
	if cr.Metadata.Extra["model"] != "opus" {
		t.Errorf("model = %q, want opus (extender overrides base)", cr.Metadata.Extra["model"])
	}
	if cr.Metadata.Effort != "high" {
		t.Errorf("effort = %q, want high (extender overrides base)", cr.Metadata.Effort)
	}
	if len(cr.Metadata.Tools) != 3 {
		t.Errorf("tools = %v, want inherited [Read Grep Glob] from base", cr.Metadata.Tools)
	}
	if _, ok := cr.Metadata.Extra["extends"]; ok {
		t.Error("extends directive should be stripped from output frontmatter")
	}
}

// TestAllAgents_Extends_MissingBase degrades to a plain agent (extends dropped)
// when no base of the target name exists.
func TestAllAgents_Extends_MissingBase(t *testing.T) {
	ct := &config.ContentTree{
		Agents: []config.ContentFile{{
			Name:     "custom",
			Content:  "solo body",
			Metadata: &config.Metadata{Extra: map[string]string{"extends": "custom"}},
		}},
	}
	got := allAgents(ct)
	if len(got) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(got))
	}
	if got[0].Content != "solo body" {
		t.Errorf("body = %q, want %q", got[0].Content, "solo body")
	}
	if got[0].Metadata != nil {
		if _, ok := got[0].Metadata.Extra["extends"]; ok {
			t.Error("extends should be stripped when base is missing")
		}
	}
}
