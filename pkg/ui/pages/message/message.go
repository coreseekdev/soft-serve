package message

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/soft-serve/pkg/ui/common"
	"github.com/charmbracelet/soft-serve/pkg/ui/components/code"
)

const (
	placeholderContent = `# Message

Coming soon...

This tab will provide chat functionality for communication with AI assistants.

Stay tuned!
`
)

// Message is the model for the Message page (placeholder for chat functionality).
type Message struct {
	common   common.Common
	code     *code.Code
}

// New creates a new Message model.
func New(c common.Common) *Message {
	m := &Message{
		common: c,
	}

	code := code.New(c, "", "")
	code.UseGlamour = true
	code.NoContentStyle = c.Styles.NoContent.SetString("Message feature coming soon...")
	m.code = code

	return m
}

// SetSize implements common.Component.
func (m *Message) SetSize(width, height int) {
	m.common.SetSize(width, height)
	m.code.SetSize(width, height-2)
}

// ShortHelp implements help.KeyMap.
func (m *Message) ShortHelp() []key.Binding {
	k := m.code.KeyMap
	return []key.Binding{
		m.common.KeyMap.Section,
		k.Up,
		k.Down,
	}
}

// FullHelp implements help.KeyMap.
func (m *Message) FullHelp() [][]key.Binding {
	k := m.code.KeyMap
	return [][]key.Binding{
		{m.common.KeyMap.Section},
		{k.Up, k.Down},
		{k.PageDown, k.PageUp},
		{k.HalfPageDown, k.HalfPageUp},
	}
}

// Init implements tea.Model.
func (m *Message) Init() tea.Cmd {
	// Set placeholder content with glamour rendering
	return m.code.SetContent(placeholderContent, "md")
}

// Update implements tea.Model.
func (m *Message) Update(msg tea.Msg) (common.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	}

	c, cmd := m.code.Update(msg)
	m.code = c.(*code.Code)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m *Message) View() string {
	// Tab indicator
	tabIndicator := m.common.Styles.TabActive.Render("Message")

	view := lipgloss.JoinVertical(lipgloss.Left,
		m.common.Styles.Tabs.Render(tabIndicator),
		m.code.View(),
	)

	return view
}
