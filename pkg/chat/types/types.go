// Package types provides shared types for the chat system.
package types

import (
	"time"
)

// UserMessageType defines the type of user mailbox message.
type UserMessageType string

const (
	// Message types
	UserMsgDM      UserMessageType = "dm"      // Direct message (in/out)
	UserMsgChannel UserMessageType = "channel" // Channel message copy (own messages)
	UserMsgMention UserMessageType = "mention" // @mention notification

	// State types
	UserMsgSub      UserMessageType = "sub"      // Subscribe to channel
	UserMsgUnsub    UserMessageType = "unsub"    // Unsubscribe from channel
	UserMsgMark     UserMessageType = "mark"     // Read mark (channels and private)
	UserMsgAck      UserMessageType = "ack"      // Delivery acknowledgment
	UserMsgSnapshot UserMessageType = "snapshot" // State snapshot
)

// UserMessage represents a message in a user's mailbox.
type UserMessage struct {
	Type      UserMessageType `json:"type"`
	ID        string          `json:"id,omitempty"`  // Unique message ID (optional for non-message types)
	Timestamp time.Time       `json:"ts"`

	// dm / channel types
	Direction string   `json:"direction,omitempty"` // "in" (received) / "out" (sent)
	Peer      string   `json:"peer,omitempty"`      // Counterparty: @user (DM) or #channel
	From      string   `json:"from,omitempty"`      // Sender (for received messages)
	Content   string   `json:"content,omitempty"`
	Mentions  []string `json:"mentions,omitempty"`

	// mention / sub / mark types
	Inbox  string `json:"inbox,omitempty"`  // Source/target: #channel or @user
	Cursor string `json:"cursor,omitempty"` // Read position

	// snapshot type
	State *UserState `json:"state,omitempty"`

	// Message threading (RFC 5322)
	ReplyTo    string   `json:"reply_to,omitempty"`
	References []string `json:"references,omitempty"`
}

// UserState represents a user's current state.
type UserState struct {
	Cursors map[string]string `json:"cursors"` // inbox -> cursor (channels and private chats)
}

// GetCursor returns the cursor for a specific inbox.
func (s *UserState) GetCursor(inbox string) string {
	if s == nil || s.Cursors == nil {
		return ""
	}
	return s.Cursors[inbox]
}

// SetCursor sets the cursor for a specific inbox.
func (s *UserState) SetCursor(inbox, cursor string) {
	if s.Cursors == nil {
		s.Cursors = make(map[string]string)
	}
	s.Cursors[inbox] = cursor
}

// ChannelMessageType defines the type of channel mailbox message.
type ChannelMessageType string

const (
	ChannelMsgMessage  ChannelMessageType = "msg"     // Channel message
	ChannelMsgJoin     ChannelMessageType = "join"    // User joined
	ChannelMsgLeave    ChannelMessageType = "leave"   // User left
	ChannelMsgTopic    ChannelMessageType = "topic"   // Topic change
	ChannelMsgMembers  ChannelMessageType = "members" // Member snapshot
	ChannelMsgSnapshot ChannelMessageType = "snapshot" // Channel state snapshot
)

// ChannelMessage represents a message in a channel's mailbox.
type ChannelMessage struct {
	Type      ChannelMessageType `json:"type"`
	ID        string             `json:"id,omitempty"`
	Timestamp time.Time          `json:"ts"`

	// msg type
	From       string   `json:"from,omitempty"`
	Content    string   `json:"content,omitempty"`
	Mentions   []string `json:"mentions,omitempty"`
	ReplyTo    string   `json:"reply_to,omitempty"`
	References []string `json:"references,omitempty"`

	// join/leave/topic types
	User  string `json:"user,omitempty"`
	Topic string `json:"topic,omitempty"`

	// members/snapshot types
	Members []string `json:"members,omitempty"`
	Date    string   `json:"date,omitempty"` // YYYY-MM-DD for members snapshot
}

// Channel represents a chat channel.
type Channel struct {
	Name      string    `json:"name"`
	Topic     string    `json:"topic,omitempty"`
	Members   []string  `json:"members,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// NewChannel creates a new channel.
func NewChannel(name string) *Channel {
	return &Channel{
		Name:      name,
		Members:   make([]string, 0),
		CreatedAt: time.Now(),
	}
}

// HasMember checks if a user is a member of the channel.
func (c *Channel) HasMember(username string) bool {
	for _, m := range c.Members {
		if m == username {
			return true
		}
	}
	return false
}

// AddMember adds a user to the channel.
func (c *Channel) AddMember(username string) bool {
	if c.HasMember(username) {
		return false
	}
	c.Members = append(c.Members, username)
	return true
}

// RemoveMember removes a user from the channel.
func (c *Channel) RemoveMember(username string) bool {
	for i, m := range c.Members {
		if m == username {
			c.Members = append(c.Members[:i], c.Members[i+1:]...)
			return true
		}
	}
	return false
}

// MemberCount returns the number of members in the channel.
func (c *Channel) MemberCount() int {
	return len(c.Members)
}

// ChannelSnapshot represents a snapshot of channel state.
type ChannelSnapshot struct {
	Topic   string   `json:"topic"`
	Members []string `json:"members"`
}

// ToSnapshot creates a snapshot of the current channel state.
func (c *Channel) ToSnapshot() *ChannelSnapshot {
	members := make([]string, len(c.Members))
	copy(members, c.Members)
	return &ChannelSnapshot{
		Topic:   c.Topic,
		Members: members,
	}
}

// DefaultChannels is the list of default channels.
var DefaultChannels = []string{"#general"}

// ReadOptions defines options for reading messages.
type ReadOptions struct {
	AfterCursor string     // Start after this message ID
	StartTime   *time.Time // Start time filter (optional)
	EndTime     *time.Time // End time filter (optional)
	Limit       int        // Max messages to return (default 100)
	Types       []string   // Filter by message types (empty = all)
	Peer        string     // Filter by peer for DM (e.g., "@bob")
}

// DefaultReadLimit is the default number of messages to return.
const DefaultReadLimit = 100

// ChatStore defines the storage interface for chat.
type ChatStore interface {
	// Channel operations
	AppendChannelMsg(channel string, msg *ChannelMessage) error
	ReadChannelMsgs(channel string, opts ReadOptions) ([]*ChannelMessage, error)
	GetChannelMsg(channel, msgID string) (*ChannelMessage, error)
	GetLastChannelMsgID(channel string) (string, error)
	RebuildChannelState(channel string) (*Channel, error)

	// User operations
	AppendUserMsg(user string, msg *UserMessage) error
	AppendUserMsgIfNotExists(user string, msg *UserMessage) (bool, error)
	ReadUserMsgs(user string, opts ReadOptions) ([]*UserMessage, error)
	GetUserMsg(user, msgID string) (*UserMessage, error)
	RebuildUserState(user string) (*UserState, error)

	// Common operations
	HasMessage(inbox string, msgID string) bool
	GetLastSnapshot(inbox string) (ts time.Time, snapshot interface{}, err error)
	CountAfter(inbox string, afterTs time.Time) int
	ListChannels() ([]string, error)
	RepairInbox(inbox string) (int, error)

	// Lifecycle
	Init() error
	Shutdown() error
}

// NewUserMessage creates a new user message with timestamp.
func NewUserMessage(msgType UserMessageType) *UserMessage {
	return &UserMessage{
		Type:      msgType,
		Timestamp: time.Now(),
	}
}

// NewChannelMessage creates a new channel message with timestamp.
func NewChannelMessage(msgType ChannelMessageType) *ChannelMessage {
	return &ChannelMessage{
		Type:      msgType,
		Timestamp: time.Now(),
	}
}
