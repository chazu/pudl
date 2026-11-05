package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chazu/pudl/internal/lister"
)

// RunInteractiveList runs the bubbletea interactive list interface. With no
// entries it does nothing: the command says so before calling it.
func RunInteractiveList(entries []lister.ListEntry, verbose bool) error {
	if len(entries) == 0 {
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
