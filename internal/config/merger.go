package config

// MergeConfigs merges a main config with a local override config
// Local config items with matching IDs override main config items
func MergeConfigs(main *Config, local *Config) *Config {
	if local == nil {
		return main
	}

	merged := &Config{
		Metadata:   main.Metadata,
		Includes:   main.Includes,
		Outputs:    main.Outputs,
		Rules:      mergeRules(main.Rules, local.Rules),
		Sections:   mergeSections(main.Sections, local.Sections),
		Agents:     mergeAgents(main.Agents, local.Agents),
		MCPServers: mergeMCPServers(main.MCPServers, local.MCPServers),
		Commands:   mergeCommands(main.Commands, local.Commands),
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
	// Add remaining local rules by name that didn't override anything
	for _, rule := range localByName {
		result = append(result, rule)
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
	// Add remaining local sections by name that didn't override anything
	for _, section := range localByName {
		result = append(result, section)
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
	// Add remaining local agents by name that didn't override anything
	for name := range localByName {
		result = append(result, localByName[name])
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

// mergeMCPServers merges MCP servers with ID-based overrides
func mergeMCPServers(main, local []MCPServer) []MCPServer {
	if len(local) == 0 {
		return main
	}

	// Create lookup map for local MCP servers by ID
	localByID := make(map[string]MCPServer)
	localByName := make(map[string]MCPServer)

	for i := range local {
		if local[i].ID != "" {
			localByID[local[i].ID] = local[i]
		} else {
			localByName[local[i].Name] = local[i]
		}
	}

	var result []MCPServer

	// Process main MCP servers - override if local has matching ID or name
	for i := range main {
		server := &main[i]
		switch {
		case server.ID != "" && localByID[server.ID].Name != "":
			// Override by ID
			result = append(result, localByID[server.ID])
			delete(localByID, server.ID)
		case localByName[server.Name].Name != "":
			// Override by name if no ID match
			result = append(result, localByName[server.Name])
			delete(localByName, server.Name)
		default:
			// No override, keep original
			result = append(result, *server)
		}
	}

	// Add remaining local MCP servers that didn't override anything
	for id := range localByID {
		result = append(result, localByID[id])
	}
	// Add remaining local MCP servers by name that didn't override anything
	for name := range localByName {
		result = append(result, localByName[name])
	}

	return result
}

// mergeCommands merges commands with ID-based overrides
func mergeCommands(main, local []Command) []Command {
	if len(local) == 0 {
		return main
	}

	// Create lookup map for local commands by ID
	localByID := make(map[string]Command)
	localByName := make(map[string]Command)

	for i := range local {
		if local[i].ID != "" {
			localByID[local[i].ID] = local[i]
		} else {
			localByName[local[i].Name] = local[i]
		}
	}

	var result []Command

	// Process main commands - override if local has matching ID or name
	for i := range main {
		cmd := &main[i]
		switch {
		case cmd.ID != "" && localByID[cmd.ID].Name != "":
			// Override by ID
			result = append(result, localByID[cmd.ID])
			delete(localByID, cmd.ID)
		case localByName[cmd.Name].Name != "":
			// Override by name if no ID match
			result = append(result, localByName[cmd.Name])
			delete(localByName, cmd.Name)
		default:
			// No override, keep original
			result = append(result, *cmd)
		}
	}

	// Add remaining local commands that didn't override anything
	for id := range localByID {
		result = append(result, localByID[id])
	}
	// Add remaining local commands by name that didn't override anything
	for name := range localByName {
		result = append(result, localByName[name])
	}

	return result
}

// MergeMCPServers is a public wrapper around mergeMCPServers for testing
func MergeMCPServers(main []MCPServer, local []MCPServer) []MCPServer {
	return mergeMCPServers(main, local)
}

// MergeCommands is a public wrapper around mergeCommands for testing
func MergeCommands(main []Command, local []Command) []Command {
	return mergeCommands(main, local)
}
