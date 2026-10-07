package mcp

import (
	"context"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

const (
	defaultPollInterval = 2 * time.Second
	// maxRetryPause caps the pause between retries of a failed rebuild.
	maxRetryPause = time.Minute
)

// Replace swaps in a freshly built catalog and updates the registered skill://
// resources to match: removed skills disappear, new and changed ones are
// (re-)registered. The SDK debounces the resulting changes into one
// notifications/resources/list_changed per connected client.
func (s *Server) Replace(next *Catalog) {
	s.catMu.Lock()
	prev := s.catalog
	s.catalog = next
	s.catMu.Unlock()

	if prev != nil {
		var gone []string
		for uri := range prev.byFile {
			if _, ok := next.byFile[uri]; !ok {
				gone = append(gone, uri)
			}
		}
		if len(gone) > 0 {
			s.mcpServer.RemoveResources(gone...)
		}
	}
	for _, skill := range next.Skills() {
		if prev != nil {
			if old, ok := prev.byName[skill.Name]; ok && old.Digest == skill.Digest {
				continue
			}
		}
		s.registerSkillFiles(skill)
	}
}

func (s *Server) registerSkillFiles(skill *CatalogSkill) {
	for i := range skill.Files {
		file := &skill.Files[i]
		s.mcpServer.AddResource(&sdkmcp.Resource{
			URI:         file.URI,
			Name:        skill.Name + "/" + file.RelPath,
			Description: skill.Description,
			MIMEType:    file.MIME,
			Size:        int64(file.Size),
			Meta: sdkmcp.Meta{
				skillsMetaPrefix + "digest": file.Digest,
				"ai-rulez/skill-digest":     skill.Digest,
			},
		}, s.readResource)
	}
}

// Watch polls the Fingerprint of the catalog's inputs and, when it changes,
// rebuilds the catalog and calls Replace. A failed rebuild keeps the previous
// catalog serving and is logged. It returns when ctx ends, or at once when the
// server was built without Rebuild and Fingerprint. Polling (not fsnotify)
// keeps the dependency set unchanged and behaves the same on every platform.
func (s *Server) Watch(ctx context.Context) {
	o := s.serve.opts
	log := logger.Or(o.Log)
	if o.Rebuild == nil || o.Fingerprint == nil {
		return
	}
	interval := o.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	last := o.Baseline
	if last == "" {
		var err error
		if last, err = o.Fingerprint(); err != nil {
			log.Warn("Live reload disabled: cannot fingerprint the skill files", "error", err.Error())
			return
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var (
		failures int
		retryAt  time.Time
		failedAt string // the fingerprint whose rebuild failed
	)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		cur, err := o.Fingerprint()
		// A new edit since the failed rebuild ends the pause: the user fixed it.
		if err != nil || cur == last || (ambient.Clock(nil).Now().Before(retryAt) && cur == failedAt) {
			continue
		}
		next, err := o.Rebuild()
		if err != nil {
			// Keep `last`: the change is still unapplied, so the rebuild is tried
			// again (with a growing pause) instead of waiting for another edit.
			failures++
			failedAt = cur
			retryAt = ambient.Clock(nil).Now().Add(min(interval<<min(failures, 6), maxRetryPause))
			log.Warn("Skill files changed but the catalog could not be rebuilt; keeping the previous one and retrying", "error", err.Error(), "attempt", failures)
			continue
		}
		last, failures, retryAt = cur, 0, time.Time{}
		s.Replace(next)
		log.Info("Reloaded served skills", "skills", len(next.Skills()))
	}
}
