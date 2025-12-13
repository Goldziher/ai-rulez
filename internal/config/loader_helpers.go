package config

func setDefaultPriorities(config *Config) {
	setRulesPriorities(config.Rules)
	setSectionsPriorities(config.Sections)
	setAgentsPriorities(config.Agents)
}

func setRulesPriorities(rules []Rule) {
	for i := range rules {
		if rules[i].Priority == "" {
			rules[i].Priority = PriorityMedium
		}
	}
}

func setSectionsPriorities(sections []Section) {
	for i := range sections {
		if sections[i].Priority == "" {
			sections[i].Priority = PriorityMedium
		}
	}
}

func setAgentsPriorities(agents []Agent) {
	for i := range agents {
		if agents[i].Priority == "" {
			agents[i].Priority = PriorityMedium
		}
	}
}
