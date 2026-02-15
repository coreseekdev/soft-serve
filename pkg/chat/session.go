package chat

import (
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/ssh"
)

// ChatSession represents an active chat session.
type ChatSession struct {
	mu     sync.Mutex
	user   *ChatUser
	chat   *Chat
	term   Terminal
	msgs   chan *UserMessage
	done   chan struct{}
	closed bool
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
		user: user,
		chat: chat,
		term: term,
		msgs: make(chan *UserMessage, 100),
		done: make(chan struct{}),
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

// Close closes the session.
func (s *ChatSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.done)
	return s.term.Close()
}

// Done returns the done channel.
func (s *ChatSession) Done() <-chan struct{} {
	return s.done
}

// Messages returns the message channel.
func (s *ChatSession) Messages() <-chan *UserMessage {
	return s.msgs
}

// PushChannelMessage pushes a channel message to the session.
func (s *ChatSession) PushChannelMessage(msg *ChannelMessage) error {
	return s.WriteLine(formatChannelMessage(msg))
}

// PushUserMessage pushes a user message to the session.
func (s *ChatSession) PushUserMessage(msg *UserMessage) error {
	return s.WriteLine(formatUserMessage(msg))
}

// SendNotification sends a notification to the session.
func (s *ChatSession) SendNotification(msg string) error {
	return s.WriteLine("\n[notification] " + msg + "\n")
}

// formatChannelMessage formats a channel message for display.
func formatChannelMessage(msg *ChannelMessage) string {
	ts := msg.Timestamp.Format("2006-01-02 15:04")
	switch msg.Type {
	case ChannelMsgMessage:
		return fmt.Sprintf("[%s] %s: %s", ts, msg.From, msg.Content)
	case ChannelMsgJoin:
		return fmt.Sprintf("[%s] *** %s joined", ts, msg.User)
	case ChannelMsgLeave:
		return fmt.Sprintf("[%s] *** %s left", ts, msg.User)
	case ChannelMsgTopic:
		return fmt.Sprintf("[%s] *** topic changed to: %s", ts, msg.Topic)
	default:
		return ""
	}
}

// formatUserMessage formats a user message for display.
func formatUserMessage(msg *UserMessage) string {
	ts := msg.Timestamp.Format("2006-01-02 15:04")
	switch msg.Type {
	case UserMsgDM:
		if msg.Direction == "in" {
			return fmt.Sprintf("[DM %s] %s: %s", ts, msg.From, msg.Content)
		}
		return fmt.Sprintf("[DM %s] -> %s: %s", ts, msg.Peer, msg.Content)
	case UserMsgMention:
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
