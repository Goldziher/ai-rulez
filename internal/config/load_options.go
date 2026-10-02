package config

// loadOptions are the resolved LoadOption settings.
type loadOptions struct {
	withoutLocal bool
	includeMemo  any
}

// LoadOption customizes how a configuration is loaded.
type LoadOption func(*loadOptions)

// WithoutLocal skips machine-local inputs: the config.local.* overlay and the
// .ai-rulez/local/ content tree. Use it for anything that reads the shared
// configuration in order to modify and save it, and to reproduce the view a
// teammate without local overrides sees.
func WithoutLocal() LoadOption {
	return func(o *loadOptions) { o.withoutLocal = true }
}

// WithIncludeMemo makes the loaded config share an include fetch cache created
// by an earlier load (Config.IncludeMemo), so sources are fetched once per run.
func WithIncludeMemo(memo any) LoadOption {
	return func(o *loadOptions) { o.includeMemo = memo }
}

func applyLoadOptions(opts []LoadOption) loadOptions {
	var lo loadOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&lo)
		}
	}
	return lo
}
