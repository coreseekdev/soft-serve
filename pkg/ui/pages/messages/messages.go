package messages

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/soft-serve/pkg/chat"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/charmbracelet/soft-serve/pkg/ui/common"
)

// Styles for the chat UI
var (
	channelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("36")).
			Bold(true)
	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
	senderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)
	senderSelfStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("36"))
)

// Messages is the model for the Messages page (IRC-style chat).
type Messages struct {
	common     common.Common
	chat       *chat.Chat
	user       proto.User
	input      textinput.Model
	messages   []ChatLine
	channels   []string
	currentCh  string // Current channel
	ready      bool
	width      int
	height     int
	inputIndex int // Cursor position in input history
	inputHist  []string
}

// ChatLine represents a single line in the chat.
type ChatLine struct {
	Timestamp time.Time
	Sender    string
	Content   string
	IsSystem  bool
	Channel   string
}

// New creates a new Messages model.
func New(c common.Common) *Messages {
	ti := textinput.New()
	ti.Placeholder = "Type a message or /help for commands"
	ti.Prompt = "> "
	ti.Focus()

	m := &Messages{
		common:    c,
		input:     ti,
		messages:  make([]ChatLine, 0),
		channels:  make([]string, 0),
		inputHist: make([]string, 0),
	}

	return m
}

// SetChat sets the chat instance.
func (m *Messages) SetChat(chat *chat.Chat) {
	m.chat = chat
}

// SetUser sets the current user.
func (m *Messages) SetUser(user proto.User) {
	m.user = user
}

// SetSize implements common.Component.
func (m *Messages) SetSize(width, height int) {
	m.common.SetSize(width, height)
	m.width = width
	m.height = height
}

// ShortHelp implements help.KeyMap.
func (m *Messages) ShortHelp() []key.Binding {
	return []key.Binding{
		m.common.KeyMap.SelectItem,
		m.common.KeyMap.BackItem,
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "cycle channels")),
	}
}

// FullHelp implements help.KeyMap.
func (m *Messages) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{m.common.KeyMap.SelectItem, m.common.KeyMap.BackItem},
		{key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send message"))},
		{key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "cycle channels"))},
		{key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "clear input"))},
		{key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "previous message"))},
		{key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "next message"))},
	}
}

// Init implements tea.Model.
func (m *Messages) Init() tea.Cmd {
	return tea.Batch(textinput.Blink)
}

// Update implements tea.Model.
func (m *Messages) Update(msg tea.Msg) (common.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.common.KeyMap.SelectItem) || msg.String() == "enter":
			return m.handleSend()
		case key.Matches(msg, m.common.KeyMap.BackItem):
			// Go back in channel list or clear input
			if m.input.Value() != "" {
				m.input.SetValue("")
			}
		case msg.String() == "tab":
			// Cycle through channels
			m.cycleChannel()
		case msg.String() == "ctrl+u":
			// Clear input
			m.input.SetValue("")
		case msg.String() == "ctrl+p":
			// Previous message in history
			m.navigateHistory(-1)
		case msg.String() == "ctrl+n":
			// Next message in history
			m.navigateHistory(1)
		default:
			// Update input
			i, cmd := m.input.Update(msg)
			m.input = i
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	// Custom messages from chat
	case ChatLineMsg:
		m.addLine(msg.Line)
	}

	return m, tea.Batch(cmds...)
}

// ChatLineMsg is a message containing a new chat line.
type ChatLineMsg struct {
	Line ChatLine
}

// handleSend handles the enter key to send a message.
func (m *Messages) handleSend() (common.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input.Value())
	if input == "" {
		return m, nil
	}

	// Add to history
	m.inputHist = append(m.inputHist, input)
	m.inputIndex = len(m.inputHist)

	// Clear input
	m.input.SetValue("")

	// Process the input
	if m.chat != nil && m.user != nil {
		go m.processInput(input)
	} else {
		// Chat not ready, show local echo
		m.addLine(ChatLine{
			Timestamp: time.Now(),
			Sender:    m.user.Username(),
			Content:   input,
			IsSystem:  false,
			Channel:   m.currentCh,
		})
		m.addLine(ChatLine{
			Timestamp: time.Now(),
			Content:   "Chat not connected. Please restart the server with chat enabled.",
			IsSystem:  true,
		})
	}

	return m, nil
}

// processInput processes user input in a goroutine.
func (m *Messages) processInput(input string) {
	// Parse command
	if strings.HasPrefix(input, "/") {
		m.handleCommand(input)
		return
	}

	// Send message to current channel
	if m.currentCh != "" {
		// Add local echo
		m.addLine(ChatLine{
			Timestamp: time.Now(),
			Sender:    m.user.Username(),
			Content:   input,
			IsSystem:  false,
			Channel:   m.currentCh,
		})
		// TODO: Actually send to chat system
	}
}

// handleCommand handles slash commands.
func (m *Messages) handleCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/help":
		m.showHelp()
	case "/join":
		if len(args) > 0 {
			m.joinChannel(args[0])
		}
	case "/leave":
		if len(args) > 0 {
			m.leaveChannel(args[0])
		}
	case "/select":
		if len(args) > 0 {
			m.selectTarget(args[0])
		}
	case "/channels":
		m.listChannels()
	case "/topic":
		if len(args) > 1 {
			m.setTopic(args[0], strings.Join(args[1:], " "))
		} else if len(args) == 1 {
			m.getTopic(args[0])
		}
	case "/who":
		m.listUsers()
	case "/dm":
		if len(args) >= 2 {
			m.sendDM(args[0], strings.Join(args[1:], " "))
		}
	default:
		m.addSystemLine("Unknown command. Type /help for available commands.")
	}
}

// showHelp shows available commands.
func (m *Messages) showHelp() {
	help := []string{
		"Available commands:",
		"  /help              - Show this help",
		"  /join #channel     - Join/create a channel",
		"  /leave #channel    - Leave a channel",
		"  /select #channel   - Switch to channel",
		"  /select @user      - Switch to DM with user",
		"  /channels          - List channels",
		"  /topic #channel    - Show channel topic",
		"  /topic #channel x  - Set channel topic",
		"  /who               - List users in channel",
		"  /dm @user message  - Send direct message",
		"  /mark              - Mark as read",
		"  /history           - Show message history",
		"  Tab                - Cycle channels",
		"  Ctrl+P/N           - Navigate input history",
	}
	for _, line := range help {
		m.addSystemLine(line)
	}
}

// joinChannel joins a channel.
func (m *Messages) joinChannel(channel string) {
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	// Check if already joined
	for _, ch := range m.channels {
		if ch == channel {
			m.selectTarget(channel)
			return
		}
	}

	m.channels = append(m.channels, channel)
	m.currentCh = channel
	m.addSystemLine(fmt.Sprintf("Joined %s", channel))
}

// leaveChannel leaves a channel.
func (m *Messages) leaveChannel(channel string) {
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	for i, ch := range m.channels {
		if ch == channel {
			m.channels = append(m.channels[:i], m.channels[i+1:]...)
			m.addSystemLine(fmt.Sprintf("Left %s", channel))
			if m.currentCh == channel {
				if len(m.channels) > 0 {
					m.currentCh = m.channels[0]
				} else {
					m.currentCh = ""
				}
			}
			return
		}
	}
}

// selectTarget selects a channel or user.
func (m *Messages) selectTarget(target string) {
	if strings.HasPrefix(target, "#") {
		m.currentCh = target
		m.addSystemLine(fmt.Sprintf("Now talking in %s", target))
	} else if strings.HasPrefix(target, "@") {
		m.currentCh = target
		m.addSystemLine(fmt.Sprintf("Now talking with %s", target))
	}
}

// listChannels lists available channels.
func (m *Messages) listChannels() {
	if len(m.channels) == 0 {
		m.addSystemLine("No channels joined. Use /join #channel to join one.")
		return
	}

	m.addSystemLine("Joined channels:")
	for _, ch := range m.channels {
		marker := " "
		if ch == m.currentCh {
			marker = "*"
		}
		m.addSystemLine(fmt.Sprintf("  %s %s", marker, ch))
	}
}

// setTopic sets a channel topic.
func (m *Messages) setTopic(channel, topic string) {
	m.addSystemLine(fmt.Sprintf("Topic for %s set to: %s", channel, topic))
}

// getTopic gets a channel topic.
func (m *Messages) getTopic(channel string) {
	m.addSystemLine(fmt.Sprintf("Topic for %s: (no topic)", channel))
}

// listUsers lists users in current channel.
func (m *Messages) listUsers() {
	if m.currentCh == "" {
		m.addSystemLine("Not in any channel.")
		return
	}
	m.addSystemLine(fmt.Sprintf("Users in %s: (not implemented)", m.currentCh))
}

// sendDM sends a direct message.
func (m *Messages) sendDM(user, message string) {
	if !strings.HasPrefix(user, "@") {
		user = "@" + user
	}

	m.addLine(ChatLine{
		Timestamp: time.Now(),
		Sender:    m.user.Username(),
		Content:   message,
		IsSystem:  false,
		Channel:   user,
	})
}

// cycleChannel cycles through channels.
func (m *Messages) cycleChannel() {
	if len(m.channels) == 0 {
		return
	}

	for i, ch := range m.channels {
		if ch == m.currentCh {
			next := (i + 1) % len(m.channels)
			m.currentCh = m.channels[next]
			return
		}
	}

	m.currentCh = m.channels[0]
}

// navigateHistory navigates input history.
func (m *Messages) navigateHistory(dir int) {
	if len(m.inputHist) == 0 {
		return
	}

	m.inputIndex += dir
	if m.inputIndex < 0 {
		m.inputIndex = 0
	}
	if m.inputIndex >= len(m.inputHist) {
		m.inputIndex = len(m.inputHist)
		m.input.SetValue("")
		return
	}

	m.input.SetValue(m.inputHist[m.inputIndex])
}

// addLine adds a chat line.
func (m *Messages) addLine(line ChatLine) {
	m.messages = append(m.messages, line)

	// Keep only last 1000 messages
	if len(m.messages) > 1000 {
		m.messages = m.messages[len(m.messages)-1000:]
	}
}

// addSystemLine adds a system message.
func (m *Messages) addSystemLine(content string) {
	m.addLine(ChatLine{
		Timestamp: time.Now(),
		Content:   content,
		IsSystem:  true,
	})
}

// View implements tea.Model.
func (m *Messages) View() string {
	if m.width == 0 {
		return ""
	}

	var sections []string

	// Channel tabs
	if len(m.channels) > 0 {
		tabs := m.renderTabs()
		sections = append(sections, tabs)
	}

	// Message area
	messages := m.renderMessages()
	sections = append(sections, messages)

	// Input area
	input := m.renderInput()
	sections = append(sections, input)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderTabs renders channel tabs.
func (m *Messages) renderTabs() string {
	if len(m.channels) == 0 {
		return ""
	}

	var tabs []string
	for _, ch := range m.channels {
		style := m.common.Styles.TopLevelNormalTab
		if ch == m.currentCh {
			style = m.common.Styles.TopLevelActiveTab
		}
		tabs = append(tabs, style.Render(ch))
	}

	return m.common.Styles.Tabs.Render(strings.Join(tabs, " "))
}

// renderMessages renders the message area.
func (m *Messages) renderMessages() string {
	// Calculate available height for messages
	msgHeight := m.height - 4 // Reserve space for tabs and input
	if msgHeight < 5 {
		msgHeight = 5
	}

	// Get last N messages that fit
	start := 0
	if len(m.messages) > msgHeight {
		start = len(m.messages) - msgHeight
	}

	var lines []string
	for i := start; i < len(m.messages); i++ {
		lines = append(lines, m.formatLine(m.messages[i]))
	}

	// Pad with empty lines if needed
	for len(lines) < msgHeight {
		lines = append([]string{""}, lines...)
	}

	content := strings.Join(lines, "\n")
	return content
}

// formatLine formats a single chat line.
func (m *Messages) formatLine(line ChatLine) string {
	ts := line.Timestamp.Format("15:04")

	if line.IsSystem {
		return fmt.Sprintf("%s %s",
			timestampStyle.Render("["+ts+"]"),
			systemStyle.Render(line.Content),
		)
	}

	senderStyle := senderStyle
	if m.user != nil && line.Sender == m.user.Username() {
		senderStyle = senderSelfStyle
	}

	// Format: [15:04] <sender> message
	return fmt.Sprintf("%s %s %s",
		timestampStyle.Render("["+ts+"]"),
		senderStyle.Render("<"+line.Sender+">"),
		line.Content,
	)
}

// renderInput renders the input area.
func (m *Messages) renderInput() string {
	// Show current target in prompt
	prompt := "> "
	if m.currentCh != "" {
		prompt = m.currentCh + "> "
	}

	inputLine := promptStyle.Render(prompt) + m.input.View()
	return inputLine
}

// Path implements common.TabComponent.
func (m *Messages) Path() string {
	if m.currentCh != "" {
		return m.currentCh
	}
	return ""
}

// TabName returns the tab name.
func (m *Messages) TabName() string {
	return "Messages"
}

// StatusBarValue returns status bar value.
func (m *Messages) StatusBarValue() string {
	if m.currentCh != "" {
		return m.currentCh
	}
	return "chat"
}

// StatusBarInfo returns status bar info.
func (m *Messages) StatusBarInfo() string {
	return fmt.Sprintf("%d channels", len(m.channels))
}

// SpinnerID returns the spinner ID (for compatibility).
func (m *Messages) SpinnerID() int {
	return 0
}
