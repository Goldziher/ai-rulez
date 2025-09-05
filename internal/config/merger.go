package config

import (
	"slices"
	"strings"
)

type Identifiable interface {
	Rule | Section | Agent | MCPServer | Command
}

func getID[T Identifiable](item T) string {
	switch v := any(item).(type) {
	case Rule:
		return v.ID
	case Section:
		return v.ID
	case Agent:
		return v.ID
	case MCPServer:
		return v.ID
	case Command:
		return v.ID
	}
	return ""
}

func getName[T Identifiable](item T) string {
	switch v := any(item).(type) {
	case Rule:
		return v.Name
	case Section:
		return v.Name
	case Agent:
		return v.Name
	case MCPServer:
		return v.Name
	case Command:
		return v.Name
	}
	return ""
}

func Merge[T Identifiable](main, local []T) []T {
	if len(local) == 0 {
		return main
	}

	localByID := make(map[string]T)
	localByName := make(map[string]T)

	for _, item := range local {
		if id := getID(item); id != "" {
			localByID[id] = item
		} else {
			localByName[getName(item)] = item
		}
	}

	var result []T

	for _, item := range main {
		id := getID(item)
		name := getName(item)

		if id != "" {
			if override, exists := localByID[id]; exists && getName(override) != "" {
				result = append(result, override)
				delete(localByID, id)
				continue
			}
		}

		if override, exists := localByName[name]; exists && getName(override) != "" {
			result = append(result, override)
			delete(localByName, name)
			continue
		}

		result = append(result, item)
	}

	for _, item := range localByID {
		result = append(result, item)
	}
	for _, item := range localByName {
		result = append(result, item)
	}

	return result
}

func RemoveDuplicates[T Identifiable](items []T) []T {
	if len(items) <= 1 {
		return items
	}

	slices.SortFunc(items, func(a, b T) int {
		aID, bID := getID(a), getID(b)
		if aID != bID {
			if aID == "" {
				return 1
			}
			if bID == "" {
				return -1
			}
			return strings.Compare(aID, bID)
		}
		return strings.Compare(getName(a), getName(b))
	})

	return slices.CompactFunc(items, func(a, b T) bool {
		aID, bID := getID(a), getID(b)
		if aID != "" && bID != "" {
			return aID == bID
		}
		return getName(a) == getName(b)
	})
}

func MergeConfigs(main *Config, local *Config) *Config {
	if local == nil {
		return main
	}

	merged := &Config{
		Metadata:   main.Metadata,
		Includes:   main.Includes,
		Outputs:    main.Outputs,
		Rules:      Merge(main.Rules, local.Rules),
		Sections:   Merge(main.Sections, local.Sections),
		Agents:     Merge(main.Agents, local.Agents),
		MCPServers: Merge(main.MCPServers, local.MCPServers),
		Commands:   Merge(main.Commands, local.Commands),
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
	return Merge(main, local)
}

func mergeSections(main, local []Section) []Section {
	return Merge(main, local)
}

func mergeAgents(main, local []Agent) []Agent {
	return Merge(main, local)
}

func MergeRules(main []Rule, local []Rule) []Rule {
	return mergeRules(main, local)
}

func MergeSections(main []Section, local []Section) []Section {
	return mergeSections(main, local)
}

func mergeMCPServers(main, local []MCPServer) []MCPServer {
	return Merge(main, local)
}

func mergeCommands(main, local []Command) []Command {
	return Merge(main, local)
}

func MergeMCPServers(main []MCPServer, local []MCPServer) []MCPServer {
	return mergeMCPServers(main, local)
}

func MergeCommands(main []Command, local []Command) []Command {
	return mergeCommands(main, local)
}
