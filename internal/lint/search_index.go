package lint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
)

const maxStaleNamed = 5

// checkSearchIndex reports a bad [search] table (AR9D0) and a committed skill
// search index that no longer matches the skills or the embedding model
// (AR9D1). Only an index_dir outside local/ is checked: the machine-local index
// is derived and never a finding.
func (r *runner) checkSearchIndex() {
	if r.cfg == nil || r.cfg.Search == nil {
		return
	}
	cfg := r.cfg.Search
	path, line := r.configLine("[search]")
	for _, p := range cfg.Validate() {
		r.add(CodeSearchConfigInvalid, path, line, "%s", p)
	}
	if len(cfg.Validate()) > 0 {
		return
	}
	resolved := cfg.Resolved()
	if !skillsearch.CommittedIndexDir(resolved.IndexDir) {
		return
	}
	dir, err := cfg.IndexPath(r.cfg.ConfigDir)
	if err != nil {
		return
	}
	idx, err := skillsearch.LoadIndex(dir)
	switch {
	case errors.Is(err, skillsearch.ErrNoIndex):
		if _, statErr := os.Stat(filepath.Join(dir, skillsearch.ManifestFile)); statErr != nil {
			r.add(CodeSearchIndexStale, path, line, "search: index_dir %q is a committed location but holds no index; run 'ai-rulez search index' and commit it", resolved.IndexDir)
			return
		}
		r.add(CodeSearchIndexStale, path, line, "search: the committed index in %q cannot be used (%v); rebuild it with 'ai-rulez search index'", resolved.IndexDir, err)
		return
	case err != nil:
		return
	}
	model := ""
	if cfg.Embeddings != nil {
		model = cfg.Embeddings.Model
	}
	if model == "" && r.cfg.LLM != nil {
		model = r.cfg.LLM.EmbeddingModel
	}
	if reason := idx.Manifest.Reason("", model, *cfg); reason != "" {
		r.add(CodeSearchIndexStale, path, line, "search: the committed index is out of date: %s; rebuild it with 'ai-rulez search index'", reason)
		return
	}
	// Only skills present in both are compared: which skills the index holds depends on the
	// serve flags it was built with (profile, delivery, sources), which validate does not know.
	// `search status` reports missing and no longer served skills against the real catalog.
	st := idx.Check(r.skillItems(), *cfg)
	if len(st.Stale) == 0 {
		return
	}
	r.add(CodeSearchIndexStale, path, line, "search: the committed index no longer matches the skills (changed: %s); rebuild it with 'ai-rulez search index'", nameList(st.Stale))
}

func nameList(ids []string) string {
	sort.Strings(ids)
	if len(ids) > maxStaleNamed {
		return fmt.Sprintf("%s and %d more", strings.Join(ids[:maxStaleNamed], ", "), len(ids)-maxStaleNamed)
	}
	return strings.Join(ids, ", ")
}

// skillItems are the project's own skills as search items, read from their SKILL.md.
func (r *runner) skillItems() []skillsearch.Item {
	var out []skillsearch.Item
	for i := range r.items {
		it := &r.items[i]
		if it.kind != kindSkill || it.isDoc || it.abs == "" {
			continue
		}
		raw, err := os.ReadFile(it.abs) //nolint:gosec // a content file of the project
		if err != nil {
			continue
		}
		id := it.cf.Name
		if it.itemDir != "" {
			id = filepath.Base(it.itemDir)
		}
		out = append(out, skillsearch.ItemFromSkill(id, it.domain, raw))
	}
	return out
}
