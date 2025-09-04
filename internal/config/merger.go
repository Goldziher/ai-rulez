package config

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

	if len(local.Outputs) > 0 {
		merged.Outputs = append(merged.Outputs, local.Outputs...)
	}
	if len(local.Includes) > 0 {
		merged.Includes = append(merged.Includes, local.Includes...)
	}

	return merged
}

func mergeRules(main, local []Rule) []Rule {
	if len(local) == 0 {
		return main
	}

	localByID := make(map[string]Rule)
	localByName := make(map[string]Rule)

	for _, rule := range local {
		if rule.ID != "" {
			localByID[rule.ID] = rule
		} else {
			localByName[rule.Name] = rule
		}
	}

	var result []Rule

	for _, rule := range main {
		switch {
		case rule.ID != "" && localByID[rule.ID].Name != "":
			result = append(result, localByID[rule.ID])
			delete(localByID, rule.ID)
		case localByName[rule.Name].Name != "":
			result = append(result, localByName[rule.Name])
			delete(localByName, rule.Name)
		default:
			result = append(result, rule)
		}
	}

	for _, rule := range localByID {
		result = append(result, rule)
	}
	for _, rule := range localByName {
		result = append(result, rule)
	}

	return result
}

func mergeSections(main, local []Section) []Section {
	if len(local) == 0 {
		return main
	}

	localByID := make(map[string]Section)
	localByName := make(map[string]Section)

	for _, section := range local {
		if section.ID != "" {
			localByID[section.ID] = section
		} else {
			localByName[section.Name] = section
		}
	}

	var result []Section

	for _, section := range main {
		switch {
		case section.ID != "" && localByID[section.ID].Name != "":
			result = append(result, localByID[section.ID])
			delete(localByID, section.ID)
		case localByName[section.Name].Name != "":
			result = append(result, localByName[section.Name])
			delete(localByName, section.Name)
		default:
			result = append(result, section)
		}
	}

	for _, section := range localByID {
		result = append(result, section)
	}
	for _, section := range localByName {
		result = append(result, section)
	}

	return result
}

func mergeAgents(main, local []Agent) []Agent {
	if len(local) == 0 {
		return main
	}

	localByID := make(map[string]Agent)
	localByName := make(map[string]Agent)

	for i := range local {
		agent := &local[i]
		if agent.ID != "" {
			localByID[agent.ID] = *agent
		} else {
			localByName[agent.Name] = *agent
		}
	}

	var result []Agent

	for i := range main {
		agent := &main[i]
		switch {
		case agent.ID != "" && localByID[agent.ID].Name != "":
			result = append(result, localByID[agent.ID])
			delete(localByID, agent.ID)
		case localByName[agent.Name].Name != "":
			result = append(result, localByName[agent.Name])
			delete(localByName, agent.Name)
		default:
			result = append(result, *agent)
		}
	}

	for id := range localByID {
		result = append(result, localByID[id])
	}
	for name := range localByName {
		result = append(result, localByName[name])
	}

	return result
}

func MergeRules(main []Rule, local []Rule) []Rule {
	return mergeRules(main, local)
}

func MergeSections(main []Section, local []Section) []Section {
	return mergeSections(main, local)
}

func mergeMCPServers(main, local []MCPServer) []MCPServer {
	if len(local) == 0 {
		return main
	}

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

	for i := range main {
		server := &main[i]
		switch {
		case server.ID != "" && localByID[server.ID].Name != "":
			result = append(result, localByID[server.ID])
			delete(localByID, server.ID)
		case localByName[server.Name].Name != "":
			result = append(result, localByName[server.Name])
			delete(localByName, server.Name)
		default:
			result = append(result, *server)
		}
	}

	for id := range localByID {
		result = append(result, localByID[id])
	}
	for name := range localByName {
		result = append(result, localByName[name])
	}

	return result
}

func mergeCommands(main, local []Command) []Command {
	if len(local) == 0 {
		return main
	}

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

	for i := range main {
		cmd := &main[i]
		switch {
		case cmd.ID != "" && localByID[cmd.ID].Name != "":
			result = append(result, localByID[cmd.ID])
			delete(localByID, cmd.ID)
		case localByName[cmd.Name].Name != "":
			result = append(result, localByName[cmd.Name])
			delete(localByName, cmd.Name)
		default:
			result = append(result, *cmd)
		}
	}

	for id := range localByID {
		result = append(result, localByID[id])
	}
	for name := range localByName {
		result = append(result, localByName[name])
	}

	return result
}

func MergeMCPServers(main []MCPServer, local []MCPServer) []MCPServer {
	return mergeMCPServers(main, local)
}

func MergeCommands(main []Command, local []Command) []Command {
	return mergeCommands(main, local)
}
