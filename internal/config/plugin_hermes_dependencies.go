package config

import (
	"fmt"

	"github.com/posit-dev/go-python-packaging/requirement"
	"github.com/samber/oops"
)

func validateHermesDependencies(p *PluginAuthoring) error {
	if p.Hermes == nil {
		return nil
	}
	for i, dependency := range p.Hermes.Dependencies {
		req, err := requirement.Parse(dependency)
		if err != nil {
			return oops.
				With("field", fmt.Sprintf("plugin.hermes.dependencies[%d]", i)).
				Hint("Use a PEP 508 requirement, such as package[extra]>=1.0,<2").
				Wrapf(err, "plugin %q has an invalid Hermes dependency at index %d", p.Name, i)
		}
		// Hermes installs directory plugins' dependencies from an index only and
		// skips direct references, so one would load without its dependency.
		if req.URL != "" {
			return oops.
				With("field", fmt.Sprintf("plugin.hermes.dependencies[%d]", i)).
				Hint("Publish the package to an index and require it by name and version").
				Errorf("plugin %q has an invalid Hermes dependency at index %d: direct references are not installed by Hermes", p.Name, i)
		}
	}
	return nil
}
