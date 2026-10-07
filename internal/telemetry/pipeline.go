package telemetry

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// LocalDirName is the machine-local directory inside the config directory where
// the usage log, salt and spool live. It is gitignored by ai-rulez.
const LocalDirName = "local"

// Pipeline is what a command builds from Settings: a Recorder wired to the
// emitters the settings allow, plus the pieces doctor and flush need.
type Pipeline struct {
	Settings Settings
	Recorder *Recorder
	// LogPath is the usage log; empty when local logging is not part of this pipeline.
	LogPath string
	// Spool and Exporter are set only when export is active.
	Spool    *Spool
	Exporter *Exporter
	// OTLP is the export emitter, set with Spool and Exporter.
	OTLP *OTLP
	Salt string
	// ConfigDirName is passed to a spawned flush so it finds the same spool.
	ConfigDirName string
	Root          string
	// Spawn starts a detached flush. nil means the default (re-exec of this binary).
	Spawn func() error
	Clock Clock
}

// BuildOptions configures Build.
type BuildOptions struct {
	Root          string
	ConfigDirName string
	Version       string
	// LocalLog appends events to the usage log. Skill events from `usage record`
	// pass false: that command already wrote the line.
	LocalLog bool
	// LogPath overrides <root>/<config dir>/local/usage.jsonl.
	LogPath string
	Getenv  func(string) string
	Clock   Clock
	// Spawn overrides the detached-flush starter (tests).
	Spawn func() error
}

// LocalDir returns <root>/<config dir>/local.
func LocalDir(root, configDirName string) string {
	return filepath.Join(root, configDirName, LocalDirName)
}

// Build wires the emitters the settings allow. With telemetry off it returns a
// Pipeline whose Record does nothing.
func Build(s Settings, o BuildOptions) *Pipeline {
	clock := o.Clock
	if clock == nil {
		clock = SystemClock
	}
	p := &Pipeline{Settings: s, Root: o.Root, ConfigDirName: o.ConfigDirName, Clock: clock, Spawn: o.Spawn}
	p.LogPath = o.LogPath
	noProject := false
	if p.LogPath == "" {
		p.LogPath = filepath.Join(LocalDir(o.Root, o.ConfigDirName), usageLogName)
		// Without an ai-rulez project here there is nowhere to keep the log, salt
		// and spool: record nothing rather than create <dir>/.ai-rulez/local.
		info, err := os.Stat(filepath.Join(o.Root, o.ConfigDirName))
		noProject = err != nil || !info.IsDir()
	}
	if !s.RecordActive() || noProject {
		p.Recorder = &Recorder{Emitter: Nop{}, Clock: clock}
		return p
	}
	saltPath := s.SaltFile
	if saltPath == "" {
		saltPath = usage.DefaultSaltPath(p.LogPath)
	}
	p.Salt = usage.LoadSalt(saltPath)

	var emitters []Emitter
	if o.LocalLog {
		emitters = append(emitters, JSONL{Path: p.LogPath})
	}
	if s.ExportActive() {
		p.Spool = &Spool{Dir: LocalDir(o.Root, o.ConfigDirName)}
		p.Exporter = &Exporter{
			Spool: p.Spool, Endpoint: s.Endpoint, Protocol: s.Protocol, HeadersEnv: s.HeadersEnv, Getenv: o.Getenv,
			Encoder: s.Encoder(o.Version),
			Now:     clock,
		}
		p.OTLP = &OTLP{Spool: p.Spool, Exporter: p.Exporter, Sample: s.Sample}
		emitters = append(emitters, p.OTLP)
	}
	p.Recorder = &Recorder{Emitter: Compose(emitters...), Clock: clock, Salt: p.Salt}
	return p
}

const usageLogName = "usage.jsonl"

// Session returns the salted hash of a raw harness session id, "" for none.
func (p *Pipeline) Session(raw string) string {
	if raw == "" || p.Salt == "" {
		return ""
	}
	return usage.HashSession(p.Salt, raw)
}

// Record emits one event and, when export is active and a flush is due, starts a
// detached one. It never waits on the network. The returned error is for logging
// on stderr; callers must not fail the harness on it.
func (p *Pipeline) Record(ctx context.Context, e Event) error {
	err := p.Recorder.Record(ctx, e)
	if err == nil && p.Spool != nil && p.Spool.SpawnDue(p.Clock(), DefaultFlushInterval) {
		spawn := p.Spawn
		if spawn == nil {
			spawn = p.spawnFlush
		}
		_ = spawn() //nolint:errcheck // a failed spawn only delays export to the next hook
	}
	return err
}

// spawnFlush re-executes this binary as `telemetry flush --background` with no
// inherited descriptors, so the hook can exit at once.
func (p *Pipeline) spawnFlush() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"telemetry", "flush", "--background", "--root", p.Root}
	if p.ConfigDirName != "" {
		args = append(args, "--config-dir", p.ConfigDirName)
	}
	cmd := exec.Command(exe, args...) //nolint:gosec // our own binary
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Flush ships what is due: it first queues the usage-log events past the cursor
// that the outbox does not hold (see Spool.CatchUp), then sends the outbox. The
// catch-up is best effort, so a busy or unreadable log never keeps the outbox from
// being sent; the flush is bounded by ctx and by MaxFlushTimeout.
func (p *Pipeline) Flush(ctx context.Context) (FlushResult, error) {
	if p.Exporter == nil || p.Spool == nil {
		return FlushResult{}, nil
	}
	if p.LogPath != "" {
		_, _ = p.Spool.CatchUp(p.LogPath, CatchUpOptions{Sample: p.Settings.Sample}) //nolint:errcheck // best effort: the outbox still ships
	}
	return p.Exporter.Flush(ctx)
}

// FlushTimeout bounds a background flush; it stays well under the flush lock's stale time.
const FlushTimeout = 8 * time.Second

// MaxFlushTimeout is the longest flush deadline `telemetry flush --timeout` may
// request. The flush lock goes stale at twice this, so a running flush is never
// mistaken for a crashed one and taken over.
const MaxFlushTimeout = 30 * time.Second

// Start begins background flushing for a long-lived process (the MCP server). A
// hook process does not call it: it spawns a detached flush instead.
func (p *Pipeline) Start() {
	if p.OTLP != nil {
		p.OTLP.Start()
	}
}

// Close makes a final flush bounded by ctx and releases resources.
func (p *Pipeline) Close(ctx context.Context) error {
	return p.Recorder.Emitter.Close(ctx)
}
