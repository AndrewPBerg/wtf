package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RepoPickerMode controls the picker's toggle and selection behavior.
type RepoPickerMode int

const (
	// RepoPickerRegister pre-selects unregistered repos, blocks toggling registered ones.
	RepoPickerRegister RepoPickerMode = iota
	// RepoPickerUnregister shows all repos as toggleable, nothing pre-selected.
	RepoPickerUnregister
)

// RepoPickerItem represents a single repo entry in the picker.
type RepoPickerItem struct {
	Name       string
	Path       string
	Registered bool // already in the registry
}

// RepoPickerResult holds the outcome of the repo picker.
type RepoPickerResult struct {
	Items []RepoPickerItem
	Quit  bool
}

// RunRepoPicker launches an interactive picker for selecting repos.
func RunRepoPicker(items []RepoPickerItem, mode RepoPickerMode) (RepoPickerResult, error) {
	if len(items) == 0 {
		return RepoPickerResult{Quit: true}, nil
	}

	m := newRepoPickerModel(items, mode)
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return RepoPickerResult{}, fmt.Errorf("running repo picker: %w", err)
	}

	return extractRepoResult(final.(repoPickerModel)), nil
}

func extractRepoResult(m repoPickerModel) RepoPickerResult {
	if m.quit {
		return RepoPickerResult{Quit: true}
	}

	var selected []RepoPickerItem
	for i, toggled := range m.toggled {
		if !toggled {
			continue
		}
		switch m.mode {
		case RepoPickerRegister:
			// Exclude already-registered items from register results.
			if !m.items[i].Registered {
				selected = append(selected, m.items[i])
			}
		case RepoPickerUnregister:
			selected = append(selected, m.items[i])
		}
	}
	return RepoPickerResult{Items: selected}
}

// Styles for repo picker.
var (
	rpHeaderStyle  = lipgloss.NewStyle().Bold(true)
	rpCursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	rpCheckStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	rpDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	rpNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	rpRegStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	rpSelectedName = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	rpWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
)

type repoPickerModel struct {
	items   []RepoPickerItem
	mode    RepoPickerMode
	cursor  int
	toggled []bool
	quit    bool
	done    bool
}

func newRepoPickerModel(items []RepoPickerItem, mode RepoPickerMode) repoPickerModel {
	toggled := make([]bool, len(items))
	if mode == RepoPickerRegister {
		// Pre-select unregistered repos.
		for i, item := range items {
			if !item.Registered {
				toggled[i] = true
			}
		}
	}
	return repoPickerModel{
		items:   items,
		mode:    mode,
		toggled: toggled,
	}
}

func (m repoPickerModel) Init() tea.Cmd {
	return nil
}

func (m repoPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.canToggle(m.cursor) {
				m.toggled[m.cursor] = !m.toggled[m.cursor]
			}
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// canToggle returns whether the item at index i can be toggled.
func (m repoPickerModel) canToggle(i int) bool {
	switch m.mode {
	case RepoPickerRegister:
		return !m.items[i].Registered
	case RepoPickerUnregister:
		return true
	}
	return true
}

func (m repoPickerModel) View() string {
	var sb strings.Builder

	header := "Select repos to register (space=toggle, enter=confirm, q=cancel)"
	if m.mode == RepoPickerUnregister {
		header = "Select repos to unregister (space=toggle, enter=confirm, q=cancel)"
	}
	sb.WriteString(rpHeaderStyle.Render(header))
	sb.WriteString("\n\n")

	// Calculate column widths for alignment.
	nameW := 0
	for _, item := range m.items {
		if len(item.Name) > nameW {
			nameW = len(item.Name)
		}
	}

	for i, item := range m.items {
		// Cursor indicator.
		pointer := "  "
		if i == m.cursor {
			pointer = rpCursorStyle.Render("▸ ")
		}

		// Checkbox.
		checkbox := m.viewCheckbox(i, item)

		// Name with padding.
		paddedName := item.Name + strings.Repeat(" ", nameW-len(item.Name))
		styledName := m.viewName(i, item, paddedName)

		// Path.
		path := rpDimStyle.Render(item.Path)

		// Tag for register mode.
		tag := ""
		if m.mode == RepoPickerRegister && item.Registered {
			tag = rpRegStyle.Render(" (registered)")
		}

		line := fmt.Sprintf("%s%s%s  %s%s", pointer, checkbox, styledName, path, tag)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m repoPickerModel) viewCheckbox(i int, item RepoPickerItem) string {
	if m.mode == RepoPickerRegister && item.Registered {
		return rpDimStyle.Render("[✔] ")
	}
	if m.toggled[i] {
		switch m.mode {
		case RepoPickerRegister:
			return rpCheckStyle.Render("[x] ")
		case RepoPickerUnregister:
			return rpWarnStyle.Render("[x] ")
		}
	}
	return rpDimStyle.Render("[ ] ")
}

func (m repoPickerModel) viewName(i int, item RepoPickerItem, paddedName string) string {
	if m.mode == RepoPickerRegister && item.Registered {
		return rpDimStyle.Render(paddedName)
	}
	if m.toggled[i] {
		switch m.mode {
		case RepoPickerRegister:
			return rpSelectedName.Render(paddedName)
		case RepoPickerUnregister:
			return rpWarnStyle.Render(paddedName)
		}
	}
	return rpNameStyle.Render(paddedName)
}
