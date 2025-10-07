package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"pudl/internal/lister"
)

// RunInteractiveList runs the bubbletea interactive list interface
func RunInteractiveList(entries []lister.ListEntry, verbose bool) error {
	if len(entries) == 0 {
		fmt.Println("No data found matching the specified criteria.")
		return nil
	}

	// Create the model
	model := NewModel(entries, verbose)

	// Create the program
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Run the program
	_, err := p.Run()
	return err
}
