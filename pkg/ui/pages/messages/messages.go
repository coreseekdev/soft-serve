package messages

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/chat"
	"github.com/charmbracelet/soft-serve/pkg/chat/types"
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

// TickMsg is a message sent periodically to check for new messages.
type TickMsg time.Time

// tickInterval is the interval at which we check for new messages.
const tickInterval = 500 * time.Millisecond

// SendMsg is sent when user sends a message
type SendMsg struct {
	Content string
}

// Messages is the model for the Messages page (IRC-style chat).
type Messages struct {
	common       common.Common
	chat         *chat.Chat
	user         proto.User
	input        textinput.Model
	messages     []ChatLine
	channels     []string
	currentCh    string // Current channel
	ready        bool
	width        int
	height       int
	inputIndex   int               // Cursor position in input history
	inputHist    []string          // Input history
	cursors      map[string]string // Last read message ID per channel (for storage)
	displayedIDs map[string]bool   // Message IDs we've already displayed
	logger       *log.Logger
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
	ti.Prompt = ""
	ti.Focus()

	m := &Messages{
		common:       c,
		input:        ti,
		messages:     make([]ChatLine, 0),
		channels:     make([]string, 0),
		inputHist:    make([]string, 0),
		cursors:      make(map[string]string),
		displayedIDs: make(map[string]bool),
		logger:       c.Logger.WithPrefix("messages"),
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
	// Get chat from context
	if m.common.Chat() != nil {
		m.chat = m.common.Chat()
		m.debugLog("chat instance found in context")
	} else {
		m.debugLog("chat instance NOT found in context")
	}

	// Get user from context
	if m.common.Backend() != nil && m.common.PublicKey() != nil {
		user, err := m.common.Backend().UserByPublicKey(m.common.Context(), m.common.PublicKey())
		if err == nil {
			m.user = user
			m.debugLog("user found", "username", user.Username())
		} else {
			m.debugLog("user NOT found", "error", err.Error())
		}
	} else {
		m.debugLog("backend or public key not available")
	}

	// Add welcome message
	m.addSystemLine("Welcome to Chat!")
	m.addSystemLine("Type /help for available commands.")

	if m.chat == nil {
		m.addSystemLine("")
		m.addSystemLine("Chat is not enabled. Set SOFT_SERVE_CHAT_ENABLED=true to enable.")
	} else {
		// Load initial channel list
		m.loadSubscribedChannels()
		m.debugLog("chat initialized", "channels", len(m.channels))
	}

	return tea.Batch(
		textinput.Blink,
		m.tickCmd(),
	)
}

// debugLog logs a message only in debug mode
func (m *Messages) debugLog(msg string, args ...interface{}) {
	if m.logger != nil {
		m.logger.Debug(msg, args...)
	}
}

// tickCmd returns a command that sends a TickMsg after tickInterval.
func (m *Messages) tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Update implements tea.Model.
func (m *Messages) Update(msg tea.Msg) (common.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case TickMsg:
		// Check for new messages in current channel
		if m.chat != nil && m.currentCh != "" {
			m.checkForNewMessages()
		}
		// Schedule next tick
		cmds = append(cmds, m.tickCmd())

	case SendMsg:
		// Handle send message from command
		return m.handleSendMsg(msg.Content)

	case tea.KeyPressMsg:
		// Log the key press for debugging
		m.debugLog("key press", "key", msg.String())

		// Check for special keys first - only handle non-printable keys here
		switch msg.String() {
		case "enter":
			m.debugLog("enter key detected")
			return m.handleSend()
		case "tab":
			m.debugLog("tab key detected")
			m.cycleChannel()
			return m, nil
		case "ctrl+u":
			m.debugLog("ctrl+u key detected")
			m.input.SetValue("")
			return m, nil
		case "ctrl+p":
			m.debugLog("ctrl+p key detected")
			m.navigateHistory(-1)
			return m, nil
		case "ctrl+n":
			m.debugLog("ctrl+n key detected")
			m.navigateHistory(1)
			return m, nil
		case "esc":
			// Clear input on escape
			if m.input.Value() != "" {
				m.input.SetValue("")
				return m, nil
			}
		case "backspace":
			// Let textinput handle backspace
			i, cmd := m.input.Update(msg)
			m.input = i
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		// Check key bindings ONLY when input is empty (navigation mode)
		// Don't intercept printable characters when user is typing
		if m.input.Value() == "" {
			if key.Matches(msg, m.common.KeyMap.BackItem) {
				// Only handle back when input is empty
				return m, nil
			}
		}

		// Default: pass to text input for all other keys
		i, cmd := m.input.Update(msg)
		m.input = i
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.debugLog("input updated", "value", m.input.Value())

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
	m.debugLog("handleSend called", "input", input)

	if input == "" {
		return m, nil
	}

	// Add to history
	m.inputHist = append(m.inputHist, input)
	m.inputIndex = len(m.inputHist)

	// Clear input
	m.input.SetValue("")

	// Return a command to process the input
	return m, func() tea.Msg {
		return SendMsg{Content: input}
	}
}

// handleSendMsg processes the sent message
func (m *Messages) handleSendMsg(input string) (common.Model, tea.Cmd) {
	m.debugLog("handleSendMsg", "input", input, "chat", m.chat != nil, "user", m.user != nil)

	// Get username safely
	username := "unknown"
	if m.user != nil {
		username = m.user.Username()
	}

	if m.chat == nil || m.user == nil {
		// Chat not ready, show local echo with error
		m.addLine(ChatLine{
			Timestamp: time.Now(),
			Sender:    username,
			Content:   input,
			IsSystem:  false,
			Channel:   m.currentCh,
		})
		m.addLine(ChatLine{
			Timestamp: time.Now(),
			Content:   "Chat not connected. Please check server configuration.",
			IsSystem:  true,
		})
		return m, nil
	}

	// Parse command
	if strings.HasPrefix(input, "/") {
		m.handleCommand(input)
		return m, nil
	}

	// Send message to current channel
	if m.currentCh != "" {
		// Validate message content
		if len(input) > 4096 {
			m.addSystemLine("Message too large (max 4096 bytes)")
			return m, nil
		}
		if strings.TrimSpace(input) == "" {
			return m, nil
		}

		// Send to chat store FIRST (before local echo)
		// This ensures the message is persisted before we display it
		store := m.chat.Store()
		if store != nil {
			// Generate message ID
			msgID := m.chat.GenerateMessageID()
			now := time.Now()

			msg := &types.ChannelMessage{
				ID:        msgID,
				Type:      types.ChannelMsgMessage,
				From:      username,
				Content:   input,
				Timestamp: now,
			}

			err := store.AppendChannelMsg(m.currentCh, msg)
			if err != nil {
				m.debugLog("failed to append message", "error", err.Error())
				m.addSystemLine(fmt.Sprintf("Failed to send message: %s", err.Error()))
				return m, nil
			}

			m.debugLog("message sent", "id", msgID, "channel", m.currentCh)

			// NOW add local echo AFTER successful write
			// This ensures consistency between storage and display
			m.addLine(ChatLine{
				Timestamp: now,
				Sender:    username,
				Content:   input,
				IsSystem:  false,
				Channel:   m.currentCh,
			})

			// Mark as displayed so we don't show it again when polling
			m.displayedIDs[msgID] = true
		}
	} else {
		m.addSystemLine("Not in any channel. Use /join #channel to join one.")
	}

	return m, nil
}

// handleCommand handles slash commands.
func (m *Messages) handleCommand(input string) {
	m.debugLog("handling command", "input", input)
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
		} else {
			m.addSystemLine("Usage: /join #channel")
		}
	case "/leave":
		if len(args) > 0 {
			m.leaveChannel(args[0])
		} else {
			m.addSystemLine("Usage: /leave #channel")
		}
	case "/select":
		if len(args) > 0 {
			m.selectTarget(args[0])
		} else {
			m.addSystemLine("Usage: /select #channel or /select @user")
		}
	case "/channels":
		m.listChannels()
	case "/topic":
		if len(args) > 1 {
			m.setTopic(args[0], strings.Join(args[1:], " "))
		} else if len(args) == 1 {
			m.getTopic(args[0])
		} else {
			m.addSystemLine("Usage: /topic #channel [topic]")
		}
	case "/who":
		m.listUsers()
	case "/dm":
		if len(args) >= 2 {
			m.sendDM(args[0], strings.Join(args[1:], " "))
		} else {
			m.addSystemLine("Usage: /dm @user message")
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
	m.debugLog("joined channel", "channel", channel)

	// Load and display recent history
	// Show recent messages (e.g., last 10) so users have context
	if m.chat != nil {
		store := m.chat.Store()
		if store != nil {
			opts := types.ReadOptions{
				Limit: 10,
				Types: []string{"msg"},
			}
			msgs, err := store.ReadChannelMsgs(channel, opts)
			if err == nil && len(msgs) > 0 {
				m.addSystemLine(fmt.Sprintf("--- Recent messages (%d) ---", len(msgs)))
				username := ""
				if m.user != nil {
					username = m.user.Username()
				}
				for _, msg := range msgs {
					// Display the message
					m.addLine(ChatLine{
						Timestamp: msg.Timestamp,
						Sender:    msg.From,
						Content:   msg.Content,
						IsSystem:  false,
						Channel:   channel,
					})
					// Mark as displayed so we don't show it again
					if msg.ID != "" {
						m.displayedIDs[msg.ID] = true
					}
				}
				m.debugLog("loaded and displayed recent messages", "channel", channel, "count", len(msgs))
			}
		}
	}
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
			m.debugLog("left channel", "channel", channel)
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
		m.debugLog("selected channel", "channel", target)
	} else if strings.HasPrefix(target, "@") {
		m.currentCh = target
		m.addSystemLine(fmt.Sprintf("Now talking with %s", target))
		m.debugLog("selected user", "user", target)
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

	username := "unknown"
	if m.user != nil {
		username = m.user.Username()
	}

	m.addLine(ChatLine{
		Timestamp: time.Now(),
		Sender:    username,
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
			m.debugLog("cycled to channel", "channel", m.currentCh)
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

// loadSubscribedChannels loads the user's subscribed channels.
func (m *Messages) loadSubscribedChannels() {
	if m.chat == nil || m.user == nil {
		m.debugLog("loadSubscribedChannels: chat or user is nil")
		return
	}

	// Get user state from chat store
	store := m.chat.Store()
	if store == nil {
		m.debugLog("loadSubscribedChannels: store is nil")
		return
	}

	state, err := store.RebuildUserState(m.user.Username())
	if err != nil || state == nil {
		m.debugLog("loadSubscribedChannels: failed to rebuild user state", "error", err)
		return
	}

	// Add subscribed channels
	for inbox := range state.Cursors {
		if strings.HasPrefix(inbox, "#") {
			m.channels = append(m.channels, inbox)
			m.cursors[inbox] = state.Cursors[inbox]
		}
	}

	// Set first channel as current
	if len(m.channels) > 0 {
		m.currentCh = m.channels[0]
		m.addSystemLine(fmt.Sprintf("Joined %s", m.currentCh))
	}

	m.debugLog("loaded subscribed channels", "count", len(m.channels))
}

// checkForNewMessages checks for new messages in the current channel.
func (m *Messages) checkForNewMessages() {
	if m.chat == nil || m.currentCh == "" || !strings.HasPrefix(m.currentCh, "#") {
		return
	}

	store := m.chat.Store()
	if store == nil {
		return
	}

	// Read recent messages
	opts := types.ReadOptions{
		Limit: 100,
		Types: []string{"msg"},
	}

	msgs, err := store.ReadChannelMsgs(m.currentCh, opts)
	if err != nil {
		m.debugLog("checkForNewMessages: error reading messages", "error", err.Error())
		return
	}

	// Get current username for filtering own messages
	username := ""
	if m.user != nil {
		username = m.user.Username()
	}

	// Add new messages to display
	newCount := 0
	for _, msg := range msgs {
		// Skip if we've already displayed this message
		if msg.ID != "" && m.displayedIDs[msg.ID] {
			continue
		}

		// Skip our own messages (already echoed locally)
		if msg.From == username {
			// Mark as displayed so we don't show it again
			if msg.ID != "" {
				m.displayedIDs[msg.ID] = true
			}
			continue
		}

		m.addLine(ChatLine{
			Timestamp: msg.Timestamp,
			Sender:    msg.From,
			Content:   msg.Content,
			IsSystem:  false,
			Channel:   m.currentCh,
		})

		// Mark as displayed
		if msg.ID != "" {
			m.displayedIDs[msg.ID] = true
		}
		newCount++
	}

	if newCount > 0 {
		m.debugLog("checkForNewMessages: found new messages", "count", newCount, "total_stored", len(msgs))
	}
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

	style := senderStyle
	if m.user != nil && line.Sender == m.user.Username() {
		style = senderSelfStyle
	}

	// Format: [15:04] <sender> message
	return fmt.Sprintf("%s %s %s",
		timestampStyle.Render("["+ts+"]"),
		style.Render("<"+line.Sender+">"),
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

	// Get the input view
	inputView := m.input.View()

	// Build the line: prompt + input
	inputLine := promptStyle.Render(prompt) + inputView
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
