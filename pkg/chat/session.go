package chat

import (
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/soft-serve/pkg/chat/types"
	"github.com/charmbracelet/ssh"
)

// Notification represents a notification about a new message.
type Notification struct {
	Channel string    `json:"channel"`          // Channel name (#general)
	MsgID   string    `json:"msg_id"`           // Message ID
	From    string    `json:"from"`             // Sender username
	Content string    `json:"content,omitempty"` // Message content (optional, for full push)
	Mention bool      `json:"mention"`          // Whether user was mentioned
	Time    time.Time `json:"time"`
}

// ChatSession represents an active chat session.
type ChatSession struct {
	mu         sync.Mutex
	user       *ChatUser
	chat       *Chat
	term       Terminal
	msgs       chan *types.UserMessage
	notify     chan Notification
	done       chan struct{}
	closed     bool
	channels   map[string]bool // Subscribed channels for this session
}

// Terminal represents a terminal interface for the chat session.
type Terminal interface {
	Write([]byte) (int, error)
	WriteString(string) (int, error)
	ReadLine() (string, error)
	Close() error
}

// NewChatSession creates a new chat session.
func NewChatSession(user *ChatUser, chat *Chat, term Terminal) *ChatSession {
	return &ChatSession{
		user:     user,
		chat:     chat,
		term:     term,
		msgs:     make(chan *types.UserMessage, 100),
		notify:   make(chan Notification, 100),
		done:     make(chan struct{}),
		channels: make(map[string]bool),
	}
}

// ID returns the session ID (user name).
func (s *ChatSession) ID() string {
	return s.user.Name
}

// User returns the session user.
func (s *ChatSession) User() *ChatUser {
	return s.user
}

// Subscribe adds a channel to this session's subscriptions.
func (s *ChatSession) Subscribe(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[channel] = true
}

// Unsubscribe removes a channel from this session's subscriptions.
func (s *ChatSession) Unsubscribe(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channels, channel)
}

// IsSubscribed checks if the session is subscribed to a channel.
func (s *ChatSession) IsSubscribed(channel string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.channels[channel]
}

// GetSubscribedChannels returns all subscribed channels.
func (s *ChatSession) GetSubscribedChannels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	channels := make([]string, 0, len(s.channels))
	for ch := range s.channels {
		channels = append(channels, ch)
	}
	return channels
}

// Notifications returns the notification channel.
func (s *ChatSession) Notifications() <-chan Notification {
	return s.notify
}

// Write writes a string to the terminal.
func (s *ChatSession) Write(msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session closed")
	}
	_, err := s.term.WriteString(msg)
	return err
}

// WriteLine writes a line to the terminal.
func (s *ChatSession) WriteLine(msg string) error {
	return s.Write(msg + "\n")
}

// ReadLine reads a line from the terminal.
func (s *ChatSession) ReadLine() (string, error) {
	return s.term.ReadLine()
}

// Close closes the session and cleans up resources.
func (s *ChatSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.done)
	// Close message channels to prevent goroutine leaks
	close(s.msgs)
	close(s.notify)
	return s.term.Close()
}

// Done returns the done channel.
func (s *ChatSession) Done() <-chan struct{} {
	return s.done
}

// Messages returns the message channel.
func (s *ChatSession) Messages() <-chan *types.UserMessage {
	return s.msgs
}

// PushChannelMessage pushes a channel message to the session.
func (s *ChatSession) PushChannelMessage(msg *types.ChannelMessage) error {
	return s.WriteLine(formatChannelMessage(msg))
}

// PushUserMessage pushes a user message to the session.
func (s *ChatSession) PushUserMessage(msg *types.UserMessage) error {
	return s.WriteLine(formatUserMessage(msg))
}

// SendNotification sends a notification to the session.
func (s *ChatSession) SendNotification(msg string) error {
	return s.WriteLine("\n[notification] " + msg + "\n")
}

// PushNotification pushes a notification to the notify channel.
// This is non-blocking - if the channel is full, the notification is dropped.
func (s *ChatSession) PushNotification(notif Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.notify <- notif:
	default:
		// Channel full, drop notification
	}
}

// PushNotificationBlocking pushes a notification and waits for it to be received.
func (s *ChatSession) PushNotificationBlocking(notif Notification) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("session closed")
	}
	s.mu.Unlock()

	select {
	case s.notify <- notif:
		return nil
	case <-s.done:
		return fmt.Errorf("session closed")
	}
}

// formatChannelMessage formats a channel message for display.
func formatChannelMessage(msg *types.ChannelMessage) string {
	ts := msg.Timestamp.Format("2006-01-02 15:04")
	switch msg.Type {
	case types.ChannelMsgMessage:
		return fmt.Sprintf("[%s] %s: %s", ts, msg.From, msg.Content)
	case types.ChannelMsgJoin:
		return fmt.Sprintf("[%s] *** %s joined", ts, msg.User)
	case types.ChannelMsgLeave:
		return fmt.Sprintf("[%s] *** %s left", ts, msg.User)
	case types.ChannelMsgTopic:
		return fmt.Sprintf("[%s] *** topic changed to: %s", ts, msg.Topic)
	default:
		return ""
	}
}

// formatUserMessage formats a user message for display.
func formatUserMessage(msg *types.UserMessage) string {
	ts := msg.Timestamp.Format("2006-01-02 15:04")
	switch msg.Type {
	case types.UserMsgDM:
		if msg.Direction == "in" {
			return fmt.Sprintf("[DM %s] %s: %s", ts, msg.From, msg.Content)
		}
		return fmt.Sprintf("[DM %s] -> %s: %s", ts, msg.Peer, msg.Content)
	case types.UserMsgMention:
		return fmt.Sprintf("[mention %s] %s in %s: %s", ts, msg.From, msg.Inbox, msg.Content)
	default:
		return ""
	}
}

// SSHSession wraps an SSH session as a Terminal.
type SSHSession struct {
	sess ssh.Session
}

// NewSSHSession creates a new SSH session wrapper.
func NewSSHSession(sess ssh.Session) *SSHSession {
	return &SSHSession{sess: sess}
}

// Write writes bytes to the SSH session.
func (s *SSHSession) Write(b []byte) (int, error) {
	return s.sess.Write(b)
}

// WriteString writes a string to the SSH session.
func (s *SSHSession) WriteString(str string) (int, error) {
	return s.sess.Write([]byte(str))
}

// ReadLine reads a line from the SSH session.
func (s *SSHSession) ReadLine() (string, error) {
	// This would need to be implemented with proper terminal handling
	// For now, return empty
	return "", nil
}

// Close closes the SSH session.
func (s *SSHSession) Close() error {
	return s.sess.Close()
}

// formatTime formats a timestamp for display.
func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}
