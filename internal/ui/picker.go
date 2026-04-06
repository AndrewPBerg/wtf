package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PickerItem represents a single selectable worktree entry.
type PickerItem struct {
	Branch string
	Path   string
	Head   string
	IsMain bool
	Repo   string // populated in global mode
}

// PickerResult holds the outcome of an interactive picker session.
type PickerResult struct {
	Items []PickerItem
	Quit  bool // user cancelled without selecting
}

// RunPicker launches an interactive terminal picker.
// If multi is true, users can toggle multiple items with space.
func RunPicker(items []PickerItem, multi bool) (PickerResult, error) {
	if len(items) == 0 {
		return PickerResult{Quit: true}, nil
	}

	m := newPickerModel(items, multi)
	// Render TUI to stderr so stdout stays clean for path output
	// (the shell wrapper captures stdout for cd).
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return PickerResult{}, fmt.Errorf("running picker: %w", err)
	}

	return extractResult(final.(pickerModel)), nil
}

// extractResult converts a finished pickerModel into a PickerResult.
func extractResult(m pickerModel) PickerResult {
	if m.quit {
		return PickerResult{Quit: true}
	}

	if m.multi {
		var selected []PickerItem
		for i, toggled := range m.toggled {
			if toggled {
				selected = append(selected, m.items[i])
			}
		}
		return PickerResult{Items: selected}
	}

	return PickerResult{Items: []PickerItem{m.items[m.cursor]}}
}

// Styles
var (
	cursorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true) // cyan bold
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	mainStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	branchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // cyan
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // dim
	headerStyle    = lipgloss.NewStyle().Bold(true)
	checkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green bold
	repoLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // dim
)

type pickerModel struct {
	items   []PickerItem
	cursor  int
	toggled []bool // multi-select toggles
	multi   bool
	quit    bool
	done    bool
}

func newPickerModel(items []PickerItem, multi bool) pickerModel {
	return pickerModel{
		items:   items,
		toggled: make([]bool, len(items)),
		multi:   multi,
	}
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ":
			if m.multi {
				m.toggled[m.cursor] = !m.toggled[m.cursor]
			}
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	var sb strings.Builder

	if m.multi {
		sb.WriteString(headerStyle.Render("Select worktrees (space=toggle, enter=confirm, q=cancel)"))
	} else {
		sb.WriteString(headerStyle.Render("Select a worktree to switch to (enter=select, q=cancel)"))
	}
	sb.WriteString("\n\n")

	// Calculate column widths for alignment.
	branchW := 0
	for _, item := range m.items {
		b := item.Branch
		if item.IsMain {
			b += " *"
		}
		if len(b) > branchW {
			branchW = len(b)
		}
	}

	for i, item := range m.items {
		branch := item.Branch
		if item.IsMain {
			branch += " *"
		}

		// Cursor indicator
		pointer := "  "
		if i == m.cursor {
			pointer = cursorStyle.Render("▸ ")
		}

		// Multi-select checkbox
		checkbox := ""
		if m.multi {
			if m.toggled[i] {
				checkbox = checkStyle.Render("[x] ")
			} else {
				checkbox = dimStyle.Render("[ ] ")
			}
		}

		// Branch name with color
		paddedBranch := branch + strings.Repeat(" ", branchW-len(branch))
		var styledBranch string
		switch {
		case i == m.cursor && m.toggled[i]:
			styledBranch = selectedStyle.Render(paddedBranch)
		case item.IsMain:
			styledBranch = mainStyle.Render(paddedBranch)
		default:
			styledBranch = branchStyle.Render(paddedBranch)
		}

		// Path and head
		path := dimStyle.Render(item.Path)
		head := ""
		if item.Head != "" {
			short := item.Head
			if len(short) > 7 {
				short = short[:7]
			}
			head = dimStyle.Render(short)
		}

		// Repo label for global mode
		repoLabel := ""
		if item.Repo != "" {
			repoLabel = repoLabelStyle.Render(" (" + item.Repo + ")")
		}

		line := fmt.Sprintf("%s%s%s  %s  %s%s", pointer, checkbox, styledBranch, path, head, repoLabel)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}
