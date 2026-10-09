package commands

// operatorNames lists the domains, profiles, includes or installed skills the
// operator finds in the project. A project that does not load has none.
func operatorNames(source string) []string {
	op, err := openOperator()
	if err != nil {
		return nil
	}
	ctx := cmdContext()
	var names []string
	switch source {
	case sourceDomains:
		items, _ := op.ListDomains(ctx) //nolint:errcheck // an unreadable project offers no names
		for _, it := range items {
			names = append(names, it.Name)
		}
	case sourceProfiles:
		items, _ := op.ListProfiles(ctx) //nolint:errcheck // an unreadable project offers no names
		for _, it := range items {
			names = append(names, it.Name)
		}
	case sourceIncludes:
		items, _ := op.ListIncludes(ctx) //nolint:errcheck // an unreadable project offers no names
		for _, it := range items {
			names = append(names, it.Name)
		}
	case sourceInstalled:
		items, _ := op.ListInstalledSkills(ctx) //nolint:errcheck // an unreadable project offers no names
		for _, it := range items {
			names = append(names, it.Name)
		}
	}
	return names
}
