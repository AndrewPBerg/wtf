package cli

import (
	"strings"
	"unicode"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

// Search consumes typing before picker shortcuts can interpret it as actions.
type pickerSearch struct {
	query   string
	editing bool
}

func (s *pickerSearch) update(key tea.KeyMsg) bool {
	if key.String() == "ctrl+c" {
		return false
	}
	if !s.editing {
		switch key.String() {
		case "/":
			s.editing = true
			return true
		case "esc":
			if s.query != "" {
				s.query = ""
				return true
			}
		}
		return false
	}
	switch key.Type {
	case tea.KeyEnter:
		s.editing = false
	case tea.KeyEsc:
		s.query, s.editing = "", false
	case tea.KeyBackspace, tea.KeyCtrlH:
		runes := []rune(s.query)
		if len(runes) > 0 {
			s.query = string(runes[:len(runes)-1])
		}
	case tea.KeyCtrlU:
		s.query = ""
	case tea.KeySpace:
		s.query += " "
	case tea.KeyRunes:
		s.query += plain(string(key.Runes))
	}
	return true
}

// Ordered, case-insensitive subsequences allow abbreviated names and messages.
func fuzzyPositions(value, query string) ([]int, bool) {
	needle := []rune(strings.TrimSpace(query))
	if len(needle) == 0 {
		return nil, true
	}
	positions := []int{}
	for i, r := range []rune(value) {
		if len(positions) < len(needle) && unicode.ToLower(r) == unicode.ToLower(needle[len(positions)]) {
			positions = append(positions, i)
		}
	}
	return positions, len(positions) == len(needle)
}

func (s pickerSearch) matches(row core.WorkspaceView) bool {
	for _, value := range []string{row.Name, workspaceDisplayName(row), row.Message} {
		if _, ok := fuzzyPositions(plain(value), s.query); ok {
			return true
		}
	}
	return false
}

func (s pickerSearch) highlight(p presentation, value string) string {
	value = plain(value)
	positions, ok := fuzzyPositions(value, s.query)
	if !ok || len(positions) == 0 {
		return value
	}
	var out strings.Builder
	next := 0
	for i, r := range []rune(value) {
		if next < len(positions) && positions[next] == i {
			out.WriteString(p.Title.Underline(true).Render(string(r)))
			next++
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func (s pickerSearch) hint() string {
	if s.editing {
		return "/" + s.query + "▏ · Enter apply · Esc clear"
	}
	if s.query != "" {
		return "/" + s.query + " · / edit · Esc clear"
	}
	return ""
}
