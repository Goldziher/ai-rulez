package config

// MergeConfigs merges a main config with a local override config
// Local config items with matching IDs override main config items
func MergeConfigs(main *Config, local *Config) *Config {
	if local == nil {
		return main
	}

	merged := &Config{
		Metadata: main.Metadata,
		Includes: main.Includes,
		Outputs:  main.Outputs,
		Rules:    mergeRules(main.Rules, local.Rules),
		Sections: mergeSections(main.Sections, local.Sections),
		Agents:   mergeAgents(main.Agents, local.Agents),
	}

	// Local outputs and includes can extend main config
	if len(local.Outputs) > 0 {
		merged.Outputs = append(merged.Outputs, local.Outputs...)
	}
	if len(local.Includes) > 0 {
		merged.Includes = append(merged.Includes, local.Includes...)
	}

	return merged
}

// mergeRules merges rules with ID-based overrides
func mergeRules(main, local []Rule) []Rule {
	if len(local) == 0 {
		return main
	}

	// Create lookup map for local rules by ID
	localByID := make(map[string]Rule)
	localByName := make(map[string]Rule)
	var localWithoutID []Rule

	for _, rule := range local {
		if rule.ID != "" {
			localByID[rule.ID] = rule
		} else {
			localByName[rule.Name] = rule
			localWithoutID = append(localWithoutID, rule)
		}
	}

	var result []Rule

	// Process main rules - override if local has matching ID or name
	for _, rule := range main {
		switch {
		case rule.ID != "" && localByID[rule.ID].Name != "":
			// Override by ID
			result = append(result, localByID[rule.ID])
			delete(localByID, rule.ID)
		case localByName[rule.Name].Name != "":
			// Override by name if no ID match
			result = append(result, localByName[rule.Name])
			delete(localByName, rule.Name)
		default:
			// No override, keep original
			result = append(result, rule)
		}
	}

	// Add remaining local rules that didn't override anything
	for _, rule := range localByID {
		result = append(result, rule)
	}
	for _, rule := range localWithoutID {
		if localByName[rule.Name].Name != "" {
			result = append(result, rule)
			delete(localByName, rule.Name)
		}
	}

	return result
}

// mergeSections merges sections with ID-based overrides
func mergeSections(main, local []Section) []Section {
	if len(local) == 0 {
		return main
	}

	// Create lookup map for local sections by ID
	localByID := make(map[string]Section)
	localByName := make(map[string]Section)
	var localWithoutID []Section

	for _, section := range local {
		if section.ID != "" {
			localByID[section.ID] = section
		} else {
			localByName[section.Name] = section
			localWithoutID = append(localWithoutID, section)
		}
	}

	var result []Section

	// Process main sections - override if local has matching ID or name
	for _, section := range main {
		switch {
		case section.ID != "" && localByID[section.ID].Name != "":
			// Override by ID
			result = append(result, localByID[section.ID])
			delete(localByID, section.ID)
		case localByName[section.Name].Name != "":
			// Override by name if no ID match
			result = append(result, localByName[section.Name])
			delete(localByName, section.Name)
		default:
			// No override, keep original
			result = append(result, section)
		}
	}

	// Add remaining local sections that didn't override anything
	for _, section := range localByID {
		result = append(result, section)
	}
	for _, section := range localWithoutID {
		if localByName[section.Name].Name != "" {
			result = append(result, section)
			delete(localByName, section.Name)
		}
	}

	return result
}

// mergeAgents merges agents with ID-based overrides
func mergeAgents(main, local []Agent) []Agent {
	if len(local) == 0 {
		return main
	}

	// Create lookup map for local agents by ID
	localByID := make(map[string]Agent)
	localByName := make(map[string]Agent)
	var localWithoutID []Agent

	for i := range local {
		agent := &local[i]
		if agent.ID != "" {
			localByID[agent.ID] = *agent
		} else {
			localByName[agent.Name] = *agent
			localWithoutID = append(localWithoutID, *agent)
		}
	}

	var result []Agent

	// Process main agents - override if local has matching ID or name
	for i := range main {
		agent := &main[i]
		switch {
		case agent.ID != "" && localByID[agent.ID].Name != "":
			// Override by ID
			result = append(result, localByID[agent.ID])
			delete(localByID, agent.ID)
		case localByName[agent.Name].Name != "":
			// Override by name if no ID match
			result = append(result, localByName[agent.Name])
			delete(localByName, agent.Name)
		default:
			// No override, keep original
			result = append(result, *agent)
		}
	}

	// Add remaining local agents that didn't override anything
	for id := range localByID {
		result = append(result, localByID[id])
	}
	for i := range localWithoutID {
		agent := &localWithoutID[i]
		if localByName[agent.Name].Name != "" {
			result = append(result, *agent)
			delete(localByName, agent.Name)
		}
	}

	return result
}

// MergeRules is a public wrapper around mergeRules for testing
func MergeRules(main []Rule, local []Rule) []Rule {
	return mergeRules(main, local)
}

// MergeSections is a public wrapper around mergeSections for testing
func MergeSections(main []Section, local []Section) []Section {
	return mergeSections(main, local)
}
