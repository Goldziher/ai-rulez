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
	// defaultRevalidateInterval is how often admission is judged again.
	defaultRevalidateInterval = time.Minute
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
// catalog serving and is logged. Every RevalidateInterval it also admits the
// current build again (Revalidate), because approvals expire and signing keys
// stop being valid with no file changing. It returns when ctx ends, or at once
// when the server has neither live reload nor revalidation. Polling (not
// fsnotify) keeps the dependency set unchanged and behaves the same on every
// platform.
func (s *Server) Watch(ctx context.Context) {
	o := s.serve.opts
	log := logger.Or(o.Log)
	reload := o.Rebuild != nil && o.Fingerprint != nil
	if !reload && o.Revalidate == nil {
		return
	}
	interval := o.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	revalidateEvery := o.RevalidateInterval
	if revalidateEvery <= 0 {
		revalidateEvery = defaultRevalidateInterval
	}
	interval = min(interval, revalidateEvery)
	last := o.Baseline
	if reload && last == "" {
		var err error
		if last, err = o.Fingerprint(); err != nil {
			log.Warn("Live reload disabled: cannot fingerprint the skill files", "error", err.Error())
			reload = false
			if o.Revalidate == nil {
				return
			}
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var (
		failures int
		retryAt  time.Time
		failedAt string // the fingerprint whose rebuild failed
	)
	admittedAt := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if reload {
			cur, err := o.Fingerprint()
			// A new edit since the failed rebuild ends the pause: the user fixed it.
			if err == nil && cur != last && (!ambient.Clock(nil).Now().Before(retryAt) || cur != failedAt) {
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
				admittedAt = time.Now()
				s.Replace(next)
				log.Info("Reloaded served skills", "skills", len(next.Skills()))
				continue
			}
		}
		if o.Revalidate != nil && time.Since(admittedAt) >= revalidateEvery {
			admittedAt = time.Now()
			s.revalidate(o.Revalidate(), log)
		}
	}
}

// revalidate swaps in a re-admitted catalog when admission changed, logging
// each skill that is no longer served or is served again.
func (s *Server) revalidate(next *Catalog, log logger.Logger) {
	if next == nil {
		return
	}
	prev := s.cat()
	changed := false
	for _, skill := range prev.Skills() {
		if r, refused := next.Refusal(skill.Name); refused {
			changed = true
			log.Warn("Refusing to serve a skill", "skill", skill.Name, "code", r.Code, "reason", r.Reason)
		}
	}
	for _, skill := range next.Skills() {
		if _, ok := prev.byName[skill.Name]; !ok {
			changed = true
			log.Info("Serving a skill that is now admitted", "skill", skill.Name)
		}
	}
	if changed {
		s.Replace(next)
	}
}
