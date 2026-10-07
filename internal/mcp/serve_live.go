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
	w := s.newWatcher()
	if w == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if w.reloadIfChanged() {
			continue
		}
		w.revalidateIfDue()
	}
}

// watcher is the state of one Watch loop.
type watcher struct {
	s        *Server
	o        ServeOptions
	log      logger.Logger
	clock    ambient.Clock
	reload   bool
	interval time.Duration
	// revalidateEvery is how often admission is judged again.
	revalidateEvery time.Duration
	last            string // the fingerprint of the catalog serving now
	failures        int
	retryAt         time.Time
	failedAt        string // the fingerprint whose rebuild failed
	admittedAt      time.Time
}

// newWatcher prepares a Watch loop; nil when there is nothing to watch.
func (s *Server) newWatcher() *watcher {
	o := s.serve.opts
	w := &watcher{
		s: s, o: o, log: logger.Or(o.Log), reload: o.Rebuild != nil && o.Fingerprint != nil,
		interval: o.PollInterval, revalidateEvery: o.RevalidateInterval, last: o.Baseline,
	}
	if w.interval <= 0 {
		w.interval = defaultPollInterval
	}
	if w.revalidateEvery <= 0 {
		w.revalidateEvery = defaultRevalidateInterval
	}
	w.interval = min(w.interval, w.revalidateEvery)
	if w.reload && w.last == "" {
		var err error
		if w.last, err = o.Fingerprint(); err != nil {
			w.log.Warn("Live reload disabled: cannot fingerprint the skill files", "error", err.Error())
			w.reload = false
		}
	}
	if !w.reload && o.Revalidate == nil {
		return nil
	}
	w.admittedAt = w.clock.Now()
	return w
}

// reloadIfChanged rebuilds the catalog when the fingerprint changed, and
// reports whether it tried.
func (w *watcher) reloadIfChanged() bool {
	if !w.reload {
		return false
	}
	cur, err := w.o.Fingerprint()
	// A new edit since the failed rebuild ends the pause: the user fixed it.
	if err != nil || cur == w.last || (w.clock.Now().Before(w.retryAt) && cur == w.failedAt) {
		return false
	}
	next, err := w.o.Rebuild()
	if err != nil {
		// Keep `last`: the change is still unapplied, so the rebuild is tried
		// again (with a growing pause) instead of waiting for another edit.
		w.failures++
		w.failedAt = cur
		w.retryAt = w.clock.Now().Add(min(w.interval<<min(w.failures, 6), maxRetryPause))
		w.log.Warn("Skill files changed but the catalog could not be rebuilt; keeping the previous one and retrying", "error", err.Error(), "attempt", w.failures)
		return true
	}
	if after, err := w.o.Fingerprint(); err == nil && after != cur {
		// The files changed while the catalog was built, so it may have read a
		// save in progress (a file truncated before it is written). Keep the
		// previous catalog; the next tick builds from the settled files.
		return true
	}
	w.last, w.failures, w.retryAt = cur, 0, time.Time{}
	w.admittedAt = w.clock.Now()
	w.s.Replace(next)
	w.log.Info("Reloaded served skills", "skills", len(next.Skills()))
	return true
}

// revalidateIfDue admits the current build again once revalidateEvery passed.
func (w *watcher) revalidateIfDue() {
	now := w.clock.Now()
	if w.o.Revalidate == nil || now.Sub(w.admittedAt) < w.revalidateEvery {
		return
	}
	w.admittedAt = now
	w.s.revalidate(w.o.Revalidate(), w.log)
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
