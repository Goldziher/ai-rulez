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
		if _, err := requirement.Parse(dependency); err != nil {
			return oops.
				With("field", fmt.Sprintf("plugin.hermes.dependencies[%d]", i)).
				Hint("Use a PEP 508 requirement, such as package[extra]>=1.0,<2").
				Wrapf(err, "plugin %q has an invalid Hermes dependency at index %d", p.Name, i)
		}
	}
	return nil
}
