package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/chazu/pudl/internal/lister"
)

// Model represents the bubbletea model for the list UI
type Model struct {
	list          list.Model
	entries       []lister.ListEntry
	quitting      bool
	err           error
	width         int
	height        int
	showHelp      bool
	verbose       bool
	showingDetail bool
	detailContent string
}

// NewModel creates a new list model
func NewModel(entries []lister.ListEntry, verbose bool) Model {
	// Convert entries to list items
	items := make([]list.Item, len(entries))
	for i, entry := range entries {
		items[i] = ListItem{
			Entry: entry,
			Index: i + 1,
		}
	}

	// Create list with custom delegate
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("170")).
		BorderLeftForeground(lipgloss.Color("170"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("243")).
		BorderLeftForeground(lipgloss.Color("170"))

	l := list.New(items, delegate, 0, 0)
	l.Title = "PUDL Data Entries"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		Padding(0, 1)

	// Set help text
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "show details with raw data"),
			),
			key.NewBinding(
				key.WithKeys("v"),
				key.WithHelp("v", "toggle verbose"),
			),
		}
	}

	return Model{
		list:          l,
		entries:       entries,
		verbose:       verbose,
		showHelp:      false,
		showingDetail: false,
		detailContent: "",
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 2) // Leave space for status
		return m, nil

	case tea.KeyMsg:
		// Handle detail view keys
		if m.showingDetail {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			default:
				// Any other key returns to list view
				m.showingDetail = false
				m.detailContent = ""
				return m, nil
			}
		}

		// Handle list view keys
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			// Show detailed view of selected item
			if selectedItem, ok := m.list.SelectedItem().(ListItem); ok {
				m.showingDetail = true
				m.detailContent = m.formatDetailedEntry(selectedItem.Entry)
				return m, nil
			}

		case "v":
			// Toggle verbose mode
			m.verbose = !m.verbose
			// Update the list items with new verbose setting
			items := make([]list.Item, len(m.entries))
			for i, entry := range m.entries {
				items[i] = ListItem{
					Entry: entry,
					Index: i + 1,
				}
			}
			m.list.SetItems(items)
			return m, nil

		case "?":
			// Toggle help
			m.showHelp = !m.showHelp
			m.list.SetShowHelp(m.showHelp)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the model
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	// Show detail view if in detail mode
	if m.showingDetail {
		return m.detailContent
	}

	// Main list view
	content := m.list.View()

	// Add status line
	statusLine := m.getStatusLine()
	if statusLine != "" {
		content += "\n" + statusLine
	}

	return content
}

// getStatusLine returns the status line with summary information
func (m Model) getStatusLine() string {
	if len(m.entries) == 0 {
		return ""
	}

	// Calculate summary stats
	totalSize := int64(0)
	totalRecords := 0
	for _, entry := range m.entries {
		totalSize += entry.SizeBytes
		totalRecords += entry.RecordCount
	}

	filtered := m.list.FilterState() == list.Filtering || m.list.FilterValue() != ""
	var statusParts []string

	if filtered {
		visibleCount := len(m.list.VisibleItems())
		statusParts = append(statusParts,
			fmt.Sprintf("Showing %d of %d entries", visibleCount, len(m.entries)))
	} else {
		statusParts = append(statusParts,
			fmt.Sprintf("%d entries", len(m.entries)))
	}

	statusParts = append(statusParts,
		fmt.Sprintf("Total: %s", formatBytes(totalSize)))
	statusParts = append(statusParts,
		fmt.Sprintf("Records: %s", formatInt(totalRecords)))

	// Add filter hint
	if !filtered {
		statusParts = append(statusParts, "Press / to filter")
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(strings.Join(statusParts, " • "))
}

// formatDetailedEntry formats a detailed view of an entry
func (m Model) formatDetailedEntry(entry lister.ListEntry) string {
	var details strings.Builder

	details.WriteString("\n=== Entry Details ===\n")
	fmt.Fprintf(&details, "Entry: %s\n", entry.Proquint)
	fmt.Fprintf(&details, "Hash: %s\n", entry.ID)
	fmt.Fprintf(&details, "Schema: %s\n", entry.Schema)
	fmt.Fprintf(&details, "Origin: %s\n", entry.Origin)
	fmt.Fprintf(&details, "Format: %s\n", entry.Format)
	fmt.Fprintf(&details, "Import Time: %s\n", entry.ImportTimestamp)
	fmt.Fprintf(&details, "Records: %d\n", entry.RecordCount)
	fmt.Fprintf(&details, "Size: %s\n", formatBytes(entry.SizeBytes))
	fmt.Fprintf(&details, "Confidence: %.2f\n", entry.Confidence)
	fmt.Fprintf(&details, "Data Path: %s\n", entry.StoredPath)
	fmt.Fprintf(&details, "Metadata Path: %s\n", entry.MetadataPath)

	if entry.Confidence < 0.8 {
		details.WriteString("⚠️  Low schema confidence - data may not match assigned schema\n")
	}

	// Show collection details
	if entry.CollectionType != nil {
		fmt.Fprintf(&details, "Type: %s", *entry.CollectionType)
		if *entry.CollectionType == "item" && entry.ItemID != nil {
			fmt.Fprintf(&details, " (Item ID: %s)", *entry.ItemID)
		}
		details.WriteString("\n")
	}

	// Show metadata
	fmt.Fprintf(&details, "\n%s\n", strings.Repeat("=", 60))
	details.WriteString("METADATA\n")
	fmt.Fprintf(&details, "%s\n", strings.Repeat("=", 60))

	metadataContent, err := os.ReadFile(entry.MetadataPath)
	if err != nil {
		fmt.Fprintf(&details, "Error reading metadata: %v\n", err)
	} else {
		// Pretty print JSON metadata
		var metadata map[string]interface{}
		if err := json.Unmarshal(metadataContent, &metadata); err != nil {
			fmt.Fprintf(&details, "Error parsing metadata: %v\n", err)
			fmt.Fprintf(&details, "%s\n", string(metadataContent))
		} else {
			prettyMetadata, err := json.MarshalIndent(metadata, "", "  ")
			if err != nil {
				fmt.Fprintf(&details, "%s\n", string(metadataContent))
			} else {
				fmt.Fprintf(&details, "%s\n", string(prettyMetadata))
			}
		}
	}

	// Show raw data
	fmt.Fprintf(&details, "\n%s\n", strings.Repeat("=", 60))
	details.WriteString("RAW DATA\n")
	fmt.Fprintf(&details, "%s\n", strings.Repeat("=", 60))

	rawContent, err := os.ReadFile(entry.StoredPath)
	if err != nil {
		fmt.Fprintf(&details, "Error reading raw data: %v\n", err)
	} else {
		// Try to pretty print based on format
		switch strings.ToLower(entry.Format) {
		case "json":
			var data interface{}
			if err := json.Unmarshal(rawContent, &data); err != nil {
				fmt.Fprintf(&details, "%s\n", string(rawContent))
			} else {
				prettyData, err := json.MarshalIndent(data, "", "  ")
				if err != nil {
					fmt.Fprintf(&details, "%s\n", string(rawContent))
				} else {
					fmt.Fprintf(&details, "%s\n", string(prettyData))
				}
			}
		case "yaml":
			var data interface{}
			if err := yaml.Unmarshal(rawContent, &data); err != nil {
				fmt.Fprintf(&details, "%s\n", string(rawContent))
			} else {
				prettyData, err := yaml.Marshal(data)
				if err != nil {
					fmt.Fprintf(&details, "%s\n", string(rawContent))
				} else {
					fmt.Fprintf(&details, "%s\n", string(prettyData))
				}
			}
		default:
			fmt.Fprintf(&details, "%s\n", string(rawContent))
		}
	}

	details.WriteString("\nPress any key to return to list...")
	return details.String()
}
