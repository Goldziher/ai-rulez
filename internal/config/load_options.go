package config

// loadOptions are the resolved LoadOption settings.
type loadOptions struct {
	withoutLocal bool
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

func applyLoadOptions(opts []LoadOption) loadOptions {
	var lo loadOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&lo)
		}
	}
	return lo
}
