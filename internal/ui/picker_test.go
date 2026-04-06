package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testItems() []PickerItem {
	return []PickerItem{
		{Branch: "main", Path: "/repo", Head: "abc1234567", IsMain: true},
		{Branch: "feature-a", Path: "/repo--feature-a", Head: "def5678901"},
		{Branch: "feature-b", Path: "/repo--feature-b", Head: "ghi9012345"},
	}
}

func TestPickerModel_Init(t *testing.T) {
	m := newPickerModel(testItems(), false)
	assert.Nil(t, m.Init())
	assert.Equal(t, 0, m.cursor)
	assert.False(t, m.quit)
	assert.False(t, m.done)
}

func TestPickerModel_NavigateDown(t *testing.T) {
	m := newPickerModel(testItems(), false)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(pickerModel)
	assert.Equal(t, 1, m.cursor)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(pickerModel)
	assert.Equal(t, 2, m.cursor)

	// Can't go past the end
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(pickerModel)
	assert.Equal(t, 2, m.cursor)
}

func TestPickerModel_NavigateUp(t *testing.T) {
	m := newPickerModel(testItems(), false)
	m.cursor = 2

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(pickerModel)
	assert.Equal(t, 1, m.cursor)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(pickerModel)
	assert.Equal(t, 0, m.cursor)

	// Can't go before the start
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(pickerModel)
	assert.Equal(t, 0, m.cursor)
}

func TestPickerModel_SingleSelect_Enter(t *testing.T) {
	m := newPickerModel(testItems(), false)
	m.cursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(pickerModel)
	assert.True(t, m.done)
	assert.False(t, m.quit)
	assert.NotNil(t, cmd) // tea.Quit
}

func TestPickerModel_Quit(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{"q", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
		{"esc", tea.KeyMsg{Type: tea.KeyEscape}},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPickerModel(testItems(), false)
			updated, cmd := m.Update(tt.key)
			fm := updated.(pickerModel)
			assert.True(t, fm.quit)
			assert.NotNil(t, cmd) // tea.Quit
		})
	}
}

func TestPickerModel_MultiSelect_Toggle(t *testing.T) {
	m := newPickerModel(testItems(), true)
	m.cursor = 1

	// Toggle on
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(pickerModel)
	assert.True(t, m.toggled[1])
	assert.False(t, m.toggled[0])
	assert.False(t, m.toggled[2])

	// Toggle off
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(pickerModel)
	assert.False(t, m.toggled[1])
}

func TestPickerModel_MultiSelect_SpaceIgnoredInSingleMode(t *testing.T) {
	m := newPickerModel(testItems(), false)
	m.cursor = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(pickerModel)
	assert.False(t, m.toggled[1])
}

func TestPickerModel_View_SingleSelect(t *testing.T) {
	m := newPickerModel(testItems(), false)
	view := m.View()

	assert.Contains(t, view, "Select a worktree to switch to")
	assert.Contains(t, view, "main *")
	assert.Contains(t, view, "feature-a")
	assert.Contains(t, view, "feature-b")
}

func TestPickerModel_View_MultiSelect(t *testing.T) {
	m := newPickerModel(testItems(), true)
	m.toggled[1] = true
	view := m.View()

	assert.Contains(t, view, "Select worktrees")
	assert.Contains(t, view, "[x]")
	assert.Contains(t, view, "[ ]")
}

func TestPickerModel_View_GlobalMode(t *testing.T) {
	items := []PickerItem{
		{Branch: "main", Path: "/repo-a", Head: "abc1234", IsMain: true, Repo: "repo-a"},
		{Branch: "feat", Path: "/repo-b--feat", Head: "def5678", Repo: "repo-b"},
	}
	m := newPickerModel(items, false)
	view := m.View()

	assert.Contains(t, view, "(repo-a)")
	assert.Contains(t, view, "(repo-b)")
}

func TestRunPicker_EmptyItems(t *testing.T) {
	result, err := RunPicker(nil, false)
	require.NoError(t, err)
	assert.True(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestPickerModel_HeadTruncation(t *testing.T) {
	items := []PickerItem{
		{Branch: "test", Path: "/test", Head: "abcdef1234567890"},
	}
	m := newPickerModel(items, false)
	view := m.View()
	// Should show truncated 7-char hash
	assert.Contains(t, view, "abcdef1")
}

func TestExtractResult_Quit(t *testing.T) {
	m := newPickerModel(testItems(), false)
	m.quit = true
	result := extractResult(m)
	assert.True(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestExtractResult_SingleSelect(t *testing.T) {
	m := newPickerModel(testItems(), false)
	m.cursor = 1
	result := extractResult(m)
	assert.False(t, result.Quit)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "feature-a", result.Items[0].Branch)
}

func TestExtractResult_MultiSelect_NoneToggled(t *testing.T) {
	m := newPickerModel(testItems(), true)
	result := extractResult(m)
	assert.False(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestExtractResult_MultiSelect_SomeToggled(t *testing.T) {
	m := newPickerModel(testItems(), true)
	m.toggled[0] = true
	m.toggled[2] = true
	result := extractResult(m)
	assert.False(t, result.Quit)
	require.Len(t, result.Items, 2)
	assert.Equal(t, "main", result.Items[0].Branch)
	assert.Equal(t, "feature-b", result.Items[1].Branch)
}

func TestExtractResult_MultiSelect_AllToggled(t *testing.T) {
	m := newPickerModel(testItems(), true)
	for i := range m.toggled {
		m.toggled[i] = true
	}
	result := extractResult(m)
	assert.False(t, result.Quit)
	require.Len(t, result.Items, 3)
}
