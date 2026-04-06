package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRepoItems() []RepoPickerItem {
	return []RepoPickerItem{
		{Name: "my-app", Path: "/home/user/projects/my-app", Registered: false},
		{Name: "my-lib", Path: "/home/user/projects/my-lib", Registered: false},
		{Name: "old-repo", Path: "/home/user/projects/old-repo", Registered: true},
	}
}

func testRegisteredItems() []RepoPickerItem {
	return []RepoPickerItem{
		{Name: "repo-a", Path: "/home/user/projects/repo-a", Registered: true},
		{Name: "repo-b", Path: "/home/user/projects/repo-b", Registered: true},
		{Name: "repo-c", Path: "/home/user/projects/repo-c", Registered: true},
	}
}

// --- Register mode tests ---

func TestRepoPickerRegister_Init(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	assert.Nil(t, m.Init())
	assert.Equal(t, 0, m.cursor)
	assert.False(t, m.quit)
	assert.False(t, m.done)
}

func TestRepoPickerRegister_PreSelectUnregistered(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	assert.True(t, m.toggled[0], "unregistered item should be pre-selected")
	assert.True(t, m.toggled[1], "unregistered item should be pre-selected")
	assert.False(t, m.toggled[2], "registered item should not be pre-selected")
}

func TestRepoPickerRegister_NavigateDown(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(repoPickerModel)
	assert.Equal(t, 1, m.cursor)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(repoPickerModel)
	assert.Equal(t, 2, m.cursor)

	// Can't go past the end.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(repoPickerModel)
	assert.Equal(t, 2, m.cursor)
}

func TestRepoPickerRegister_NavigateUp(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	m.cursor = 2

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(repoPickerModel)
	assert.Equal(t, 1, m.cursor)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(repoPickerModel)
	assert.Equal(t, 0, m.cursor)

	// Can't go before the start.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(repoPickerModel)
	assert.Equal(t, 0, m.cursor)
}

func TestRepoPickerRegister_Toggle(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	// Item 0 is pre-selected, toggle off.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(repoPickerModel)
	assert.False(t, m.toggled[0])

	// Toggle back on.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(repoPickerModel)
	assert.True(t, m.toggled[0])
}

func TestRepoPickerRegister_ToggleRegisteredBlocked(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	m.cursor = 2 // registered item

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(repoPickerModel)
	assert.False(t, m.toggled[2], "registered items should not be toggleable in register mode")
}

func TestRepoPickerRegister_Enter(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(repoPickerModel)
	assert.True(t, m.done)
	assert.False(t, m.quit)
	assert.NotNil(t, cmd)
}

func TestRepoPickerRegister_Quit(t *testing.T) {
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
			m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
			updated, cmd := m.Update(tt.key)
			fm := updated.(repoPickerModel)
			assert.True(t, fm.quit)
			assert.NotNil(t, cmd)
		})
	}
}

func TestRepoPickerRegister_View(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	view := m.View()

	assert.Contains(t, view, "Select repos to register")
	assert.Contains(t, view, "my-app")
	assert.Contains(t, view, "my-lib")
	assert.Contains(t, view, "old-repo")
	assert.Contains(t, view, "(registered)")
}

func TestRepoPickerRegister_View_CheckboxStates(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	view := m.View()

	assert.Contains(t, view, "[x]")
	assert.Contains(t, view, "[✔]")
}

func TestExtractRepoResult_Register_Quit(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	m.quit = true
	result := extractRepoResult(m)
	assert.True(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestExtractRepoResult_Register_SelectedUnregistered(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	// Items 0 and 1 are pre-toggled (unregistered).
	result := extractRepoResult(m)
	assert.False(t, result.Quit)
	require.Len(t, result.Items, 2)
	assert.Equal(t, "my-app", result.Items[0].Name)
	assert.Equal(t, "my-lib", result.Items[1].Name)
}

func TestExtractRepoResult_Register_RegisteredExcluded(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	// Force-toggle the registered item (shouldn't happen via UI, but test the guard).
	m.toggled[2] = true
	result := extractRepoResult(m)
	assert.False(t, result.Quit)
	require.Len(t, result.Items, 2, "registered items should be excluded from result")
}

func TestExtractRepoResult_Register_NoneToggled(t *testing.T) {
	m := newRepoPickerModel(testRepoItems(), RepoPickerRegister)
	m.toggled[0] = false
	m.toggled[1] = false
	result := extractRepoResult(m)
	assert.False(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestRunRepoPicker_EmptyItems(t *testing.T) {
	result, err := RunRepoPicker(nil, RepoPickerRegister)
	require.NoError(t, err)
	assert.True(t, result.Quit)
	assert.Empty(t, result.Items)
}

// --- Unregister mode tests ---

func TestRepoPickerUnregister_NothingPreSelected(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	for i := range m.toggled {
		assert.False(t, m.toggled[i], "nothing should be pre-selected in unregister mode")
	}
}

func TestRepoPickerUnregister_AllToggleable(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	for i := range m.items {
		assert.True(t, m.canToggle(i), "all items should be toggleable in unregister mode")
	}
}

func TestRepoPickerUnregister_Toggle(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)

	// Toggle on.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(repoPickerModel)
	assert.True(t, m.toggled[0])

	// Toggle off.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(repoPickerModel)
	assert.False(t, m.toggled[0])
}

func TestRepoPickerUnregister_View(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	view := m.View()

	assert.Contains(t, view, "Select repos to unregister")
	assert.Contains(t, view, "repo-a")
	assert.Contains(t, view, "repo-b")
	assert.Contains(t, view, "repo-c")
	assert.NotContains(t, view, "(registered)", "unregister mode should not show registered tag")
}

func TestRepoPickerUnregister_View_ToggledShowsWarning(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	m.toggled[0] = true
	view := m.View()

	// Toggled items in unregister mode use warning (red) style [x].
	assert.Contains(t, view, "[x]")
	assert.Contains(t, view, "[ ]")
}

func TestExtractRepoResult_Unregister_Quit(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	m.quit = true
	result := extractRepoResult(m)
	assert.True(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestExtractRepoResult_Unregister_SelectedItems(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	m.toggled[0] = true
	m.toggled[2] = true
	result := extractRepoResult(m)
	assert.False(t, result.Quit)
	require.Len(t, result.Items, 2)
	assert.Equal(t, "repo-a", result.Items[0].Name)
	assert.Equal(t, "repo-c", result.Items[1].Name)
}

func TestExtractRepoResult_Unregister_NoneToggled(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	result := extractRepoResult(m)
	assert.False(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestRunRepoPicker_Unregister_EmptyItems(t *testing.T) {
	result, err := RunRepoPicker(nil, RepoPickerUnregister)
	require.NoError(t, err)
	assert.True(t, result.Quit)
	assert.Empty(t, result.Items)
}

func TestRepoPickerUnregister_Navigate(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(repoPickerModel)
	assert.Equal(t, 1, m.cursor)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(repoPickerModel)
	assert.Equal(t, 0, m.cursor)
}

func TestRepoPickerUnregister_Quit(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	fm := updated.(repoPickerModel)
	assert.True(t, fm.quit)
	assert.NotNil(t, cmd)
}

func TestRepoPickerUnregister_Enter(t *testing.T) {
	m := newRepoPickerModel(testRegisteredItems(), RepoPickerUnregister)
	m.toggled[1] = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := updated.(repoPickerModel)
	assert.True(t, fm.done)
	assert.NotNil(t, cmd)
}
