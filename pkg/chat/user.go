package chat

import (
	"strings"

	"github.com/charmbracelet/soft-serve/pkg/chat/types"
)

// TargetType represents the type of chat target.
type TargetType int

const (
	TargetTypeChannel TargetType = iota // Channel (#xxx)
	TargetTypePrivate                   // Private chat (@user)
)

// ChatUser represents a user in the chat system.
type ChatUser struct {
	Name        string        // Username (from SSH auth)
	PublicKey   string        // Public key fingerprint
	CurrentType TargetType    // Current target type
	CurrentCh   string        // Current channel (e.g., "#general")
	CurrentPeer string        // Current private chat peer (e.g., "bob")
	State       *types.UserState // User state (from mailbox)
}

// NewChatUser creates a new chat user.
func NewChatUser(name string) *ChatUser {
	return &ChatUser{
		Name:  name,
		State: &types.UserState{Cursors: make(map[string]string)},
	}
}

// CurrentTarget returns the current target string.
func (u *ChatUser) CurrentTarget() string {
	if u.CurrentType == TargetTypeChannel {
		return u.CurrentCh
	}
	if u.CurrentPeer != "" {
		return "@" + u.CurrentPeer
	}
	return ""
}

// SetCurrentChannel sets the current target to a channel.
func (u *ChatUser) SetCurrentChannel(channel string) {
	u.CurrentType = TargetTypeChannel
	u.CurrentCh = channel
	u.CurrentPeer = ""
}

// SetCurrentPeer sets the current target to a private chat.
func (u *ChatUser) SetCurrentPeer(peer string) {
	u.CurrentType = TargetTypePrivate
	u.CurrentPeer = peer
	u.CurrentCh = ""
}

// GetCursor returns the cursor for a specific inbox.
func (u *ChatUser) GetCursor(inbox string) string {
	return u.State.GetCursor(inbox)
}

// SetCursor sets the cursor for a specific inbox.
func (u *ChatUser) SetCursor(inbox, cursor string) {
	u.State.SetCursor(inbox, cursor)
}

// IsSubscribed checks if the user is subscribed to a channel.
func (u *ChatUser) IsSubscribed(channel string) bool {
	_, ok := u.State.Cursors[channel]
	return ok
}

// ParseTarget parses a target string and returns the type and value.
func ParseTarget(target string) (TargetType, string) {
	if strings.HasPrefix(target, "#") {
		return TargetTypeChannel, target
	}
	if strings.HasPrefix(target, "@") {
		return TargetTypePrivate, strings.TrimPrefix(target, "@")
	}
	return TargetTypeChannel, "#" + target
}
