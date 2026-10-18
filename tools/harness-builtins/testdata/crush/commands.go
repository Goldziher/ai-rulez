// Excerpt of https://github.com/charmbracelet/crush/blob/v0.98.1/internal/ui/dialog/commands.go (lines 420-423, 448-455, 545-549, 566-571)
				}
			}
			item := NewCommandItem(c.com.Styles, "custom_"+cmd.ID, cmd.Name, "", action)
			if cmd.Skill != nil {
// ...
// defaultCommands returns the list of default system commands.
func (c *Commands) defaultCommands() []*CommandItem {
	commands := []*CommandItem{
		NewCommandItem(c.com.Styles, "new_session", "New Session", "ctrl+n", ActionNewSession{}).WithAliases("clear"),
		NewCommandItem(c.com.Styles, "switch_session", "Sessions", "ctrl+s", ActionOpenDialog{SessionsID}),
		NewCommandItem(c.com.Styles, "switch_model", "Switch Model", "ctrl+l", ActionOpenDialog{ModelsID}),
	}

// ...
		commands,
		NewCommandItem(c.com.Styles, "toggle_yolo", "Toggle Yolo Mode", "ctrl+y", ActionToggleYoloMode{}),
		NewCommandItem(c.com.Styles, "toggle_help", "Toggle Help", "ctrl+g", ActionToggleHelp{}),
		NewCommandItem(c.com.Styles, "init", "Initialize Project", "", ActionInitializeProject{}),
	)
// ...

	commands = append(
		commands,
		NewCommandItem(c.com.Styles, "quit", "Quit", "ctrl+c", tea.QuitMsg{}).WithAliases("exit"),
	)

