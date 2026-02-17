package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/soft-serve/pkg/chat/store/chatfile"
	"github.com/charmbracelet/soft-serve/pkg/config"
)

// contextKey is the key type for context values.
type contextKey struct{}

// chatContextKey is the context key for the chat instance.
var chatContextKey = contextKey{}

// ContextKey returns the context key for the chat instance.
// This is useful for setting the chat in SSH session contexts.
func ContextKey() interface{} {
	return chatContextKey
}

// WithContext returns a new context with the chat instance attached.
func WithContext(ctx context.Context, c *Chat) context.Context {
	return context.WithValue(ctx, chatContextKey, c)
}

// FromContext returns the chat instance from the context.
func FromContext(ctx context.Context) *Chat {
	if c, ok := ctx.Value(chatContextKey).(*Chat); ok {
		return c
	}
	return nil
}

// Chat is the main chat system.
type Chat struct {
	mu       sync.RWMutex
	store    ChatStore
	userList UserList
	channels map[string]*Channel
	users    map[string]*ChatUser
	sessions map[string]*ChatSession
	pushMgr  *PushManager

	// Message ID generation
	msgIDSeq int64
	msgIDMu  sync.Mutex

	// Configuration
	cfg *config.ChatConfig

	// Shutdown state
	shuttingDown bool
}

// New creates a new Chat instance.
func New(cfg *config.Config) (*Chat, error) {
	chatCfg := cfg.Chat
	if chatCfg.DataPath == "" {
		chatCfg.DataPath = "data/chat"
	}

	store, err := chatfile.NewFileStore(chatCfg.DataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat store: %w", err)
	}

	// Initialize store
	if err := store.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize chat store: %w", err)
	}

	// Create user list
	var userList UserList
	if chatCfg.Users != "" {
		userList = NewEnvUserList(chatCfg.Users)
	} else {
		userList = NewMapUserList([]string{})
	}

	c := &Chat{
		store:    store,
		userList: userList,
		channels: make(map[string]*Channel),
		users:    make(map[string]*ChatUser),
		sessions: make(map[string]*ChatSession),
		cfg:      &chatCfg,
		pushMgr:  NewPushManager(),
	}

	// Rebuild channel states
	if err := c.rebuildChannels(); err != nil {
		return nil, fmt.Errorf("failed to rebuild channels: %w", err)
	}

	return c, nil
}

// rebuildChannels rebuilds all channel states from storage.
func (c *Chat) rebuildChannels() error {
	channels, err := c.store.ListChannels()
	if err != nil {
		return err
	}

	for _, name := range channels {
		ch, err := c.store.RebuildChannelState(name)
		if err != nil {
			return fmt.Errorf("failed to rebuild channel %s: %w", name, err)
		}
		if ch != nil {
			c.channels[name] = ch
		}
	}

	return nil
}

// GenerateMessageID generates a globally unique message ID.
// Format: YYYYMMDDHHmmss-NNN
func (c *Chat) GenerateMessageID() string {
	c.msgIDMu.Lock()
	defer c.msgIDMu.Unlock()
	c.msgIDSeq++
	return fmt.Sprintf("%s-%03d", time.Now().Format("20060102150405"), c.msgIDSeq%1000)
}

// Store returns the chat store.
func (c *Chat) Store() ChatStore {
	return c.store
}

// IsValidUser checks if a user exists.
func (c *Chat) IsValidUser(name string) bool {
	return c.userList.Contains(name)
}

// GetUserList returns all valid users.
func (c *Chat) GetUserList() []string {
	return c.userList.List()
}

// GetUserListMap returns a map of valid users for quick lookup.
func (c *Chat) GetUserListMap() map[string]bool {
	users := c.userList.List()
	m := make(map[string]bool, len(users))
	for _, u := range users {
		m[u] = true
	}
	return m
}

// GetChannel returns a channel by name.
func (c *Chat) GetChannel(name string) (*Channel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ch, ok := c.channels[name]
	return ch, ok
}

// GetOrCreateChannel gets or creates a channel.
func (c *Chat) GetOrCreateChannel(name string) (*Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, ok := c.channels[name]; ok {
		return ch, nil
	}

	// Create new channel
	ch := NewChannel(name)
	c.channels[name] = ch

	// Write join event
	if err := c.store.AppendChannelMsg(name, &ChannelMessage{
		Type:      ChannelMsgSnapshot,
		Timestamp: time.Now(),
		Topic:     "",
		Members:   []string{},
	}); err != nil {
		return nil, err
	}

	return ch, nil
}

// GetOnlineUsers returns all online users.
func (c *Chat) GetOnlineUsers() []*ChatUser {
	c.mu.RLock()
	defer c.mu.RUnlock()
	users := make([]*ChatUser, 0, len(c.users))
	for _, u := range c.users {
		users = append(users, u)
	}
	return users
}

// AddOnlineUser adds a user to the online list.
func (c *Chat) AddOnlineUser(user *ChatUser) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users[user.Name] = user
}

// RemoveOnlineUser removes a user from the online list.
func (c *Chat) RemoveOnlineUser(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.users, name)
}

// IsUserOnline checks if a user is online.
func (c *Chat) IsUserOnline(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.users[name]
	return ok
}

// GetSession returns a session by ID.
func (c *Chat) GetSession(id string) (*ChatSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sess, ok := c.sessions[id]
	return sess, ok
}

// AddSession adds a session.
func (c *Chat) AddSession(sess *ChatSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions[sess.ID()] = sess
	c.users[sess.user.Name] = sess.user

	// Associate session with subscribed channels using new Subscriptions map
	if sess.user.State != nil && sess.user.State.Subscriptions != nil {
		for inbox := range sess.user.State.Subscriptions {
			if strings.HasPrefix(inbox, "#") {
				c.pushMgr.Subscribe(inbox, sess)
			}
		}
	}
	// Fallback to legacy Cursors for backwards compatibility
	if sess.user.State != nil && sess.user.State.Cursors != nil {
		for inbox := range sess.user.State.Cursors {
			if strings.HasPrefix(inbox, "#") {
				if sess.user.State.Subscriptions == nil {
					c.pushMgr.Subscribe(inbox, sess)
				}
			}
		}
	}
}

// RemoveSession removes a session.
func (c *Chat) RemoveSession(sess *ChatSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, sess.ID())

	// Remove from online users if no more sessions
	if _, ok := c.sessions[sess.user.Name]; !ok {
		delete(c.users, sess.user.Name)
	}

	// Unsubscribe from all channels
	c.pushMgr.UnsubscribeAll(sess)
}

// Shutdown gracefully shuts down the chat system.
func (c *Chat) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	c.shuttingDown = true
	c.mu.Unlock()

	// Notify all sessions
	for _, sess := range c.sessions {
		sess.Write("\nServer is shutting down. Goodbye!\n")
		sess.Close()
	}

	// Wait for sessions to close or timeout
	done := make(chan struct{})
	go func() {
		for {
			c.mu.RLock()
			count := len(c.sessions)
			c.mu.RUnlock()
			if count == 0 {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	// Write final snapshots
	c.mu.Lock()
	defer c.mu.Unlock()

	// Channel snapshots
	for name, ch := range c.channels {
		c.store.AppendChannelMsg(name, &ChannelMessage{
			Type:    ChannelMsgSnapshot,
			Topic:   ch.Topic,
			Members: ch.Members,
			Timestamp: time.Now(),
		})
	}

	// User snapshots
	for name, user := range c.users {
		c.store.AppendUserMsg(name, &UserMessage{
			Type:      UserMsgSnapshot,
			State:     user.State,
			Timestamp: time.Now(),
		})
	}

	return c.store.Shutdown()
}
