// Package chatfile provides file-based storage for chat.
package chatfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/chat/types"
)

// FileStore implements types.ChatStore using file-based storage.
type FileStore struct {
	mu       sync.RWMutex
	dataPath string
	locks    *LockManager
	logger   *log.Logger
}

// NewFileStore creates a new file-based store.
func NewFileStore(dataPath string) (*FileStore, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return &FileStore{
		dataPath: dataPath,
		locks:    NewLockManager(),
		logger:   log.Default().WithPrefix("chatfile"),
	}, nil
}

// debugLog logs a debug message if debug mode is enabled
func (s *FileStore) debugLog(msg string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Debug(msg, args...)
	}
}

// inboxPath returns the file path for an inbox.
func (s *FileStore) inboxPath(inbox string) string {
	return filepath.Join(s.dataPath, encodeInboxName(inbox)+".jsonl")
}

// encodeInboxName encodes an inbox name for use as a filename.
func encodeInboxName(name string) string {
	// Replace # with %23 and @ with %40
	name = strings.ReplaceAll(name, "#", "%23")
	name = strings.ReplaceAll(name, "@", "%40")
	return name
}

// decodeInboxName decodes an inbox name from a filename.
func decodeInboxName(encoded string) string {
	encoded = strings.ReplaceAll(encoded, "%23", "#")
	encoded = strings.ReplaceAll(encoded, "%40", "@")
	return encoded
}

// Init initializes the store.
func (s *FileStore) Init() error {
	// Create default channels if they don't exist
	for _, ch := range types.DefaultChannels {
		path := s.inboxPath(ch)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// Create empty channel with snapshot
			snapshot := &types.ChannelMessage{
				Type:      types.ChannelMsgSnapshot,
				Timestamp: time.Now(),
				Topic:     "",
				Members:   []string{},
			}
			if err := s.AppendChannelMsg(ch, snapshot); err != nil {
				return fmt.Errorf("failed to create default channel %s: %w", ch, err)
			}
		}
	}
	return nil
}

// Shutdown shuts down the store.
func (s *FileStore) Shutdown() error {
	return nil
}

// AppendChannelMsg appends a message to a channel.
func (s *FileStore) AppendChannelMsg(channel string, msg *types.ChannelMessage) error {
	s.locks.Lock(channel)
	defer s.locks.Unlock(channel)

	path := s.inboxPath(channel)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open channel file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	_, err = f.Write(append(data, '\n'))
	if err == nil {
		s.debugLog("appended message to channel", "channel", channel, "id", msg.ID, "from", msg.From, "type", msg.Type)
	}
	return err
}

// ReadChannelMsgs reads messages from a channel.
func (s *FileStore) ReadChannelMsgs(channel string, opts types.ReadOptions) ([]*types.ChannelMessage, error) {
	s.locks.RLock(channel)
	defer s.locks.RUnlock(channel)

	path := s.inboxPath(channel)
	messages, err := readFileLines(path)
	if err != nil {
		return nil, err
	}

	s.debugLog("reading channel messages", "channel", channel, "total_lines", len(messages), "after_cursor", opts.AfterCursor, "limit", opts.Limit)

	var result []*types.ChannelMessage
	for _, line := range messages {
		var msg types.ChannelMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}

		// Apply filters
		if !s.matchesFilters(&msg, opts) {
			continue
		}

		// Apply cursor filter
		if opts.AfterCursor != "" && msg.ID <= opts.AfterCursor {
			continue
		}

		result = append(result, &msg)

		// Apply limit
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
	}

	s.debugLog("read channel messages result", "channel", channel, "returned", len(result))
	return result, nil
}

// matchesFilters checks if a message matches the filter options.
func (s *FileStore) matchesFilters(msg *types.ChannelMessage, opts types.ReadOptions) bool {
	// Type filter
	if len(opts.Types) > 0 {
		match := false
		for _, t := range opts.Types {
			if string(msg.Type) == t {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Time filters
	if opts.StartTime != nil && msg.Timestamp.Before(*opts.StartTime) {
		return false
	}
	if opts.EndTime != nil && msg.Timestamp.After(*opts.EndTime) {
		return false
	}

	return true
}

// GetChannelMsg gets a specific message from a channel.
func (s *FileStore) GetChannelMsg(channel, msgID string) (*types.ChannelMessage, error) {
	msgs, err := s.ReadChannelMsgs(channel, types.ReadOptions{})
	if err != nil {
		return nil, err
	}

	for _, msg := range msgs {
		if msg.ID == msgID {
			return msg, nil
		}
	}

	return nil, nil
}

// GetLastChannelMsgID gets the last message ID from a channel.
func (s *FileStore) GetLastChannelMsgID(channel string) (string, error) {
	s.locks.RLock(channel)
	defer s.locks.RUnlock(channel)

	path := s.inboxPath(channel)
	lines, err := readFileLinesReverse(path, 1)
	if err != nil {
		return "", err
	}

	for _, line := range lines {
		var msg types.ChannelMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.ID != "" {
			return msg.ID, nil
		}
	}

	return "", nil
}

// RebuildChannelState rebuilds the channel state from storage.
func (s *FileStore) RebuildChannelState(channel string) (*types.Channel, error) {
	msgs, err := s.ReadChannelMsgs(channel, types.ReadOptions{})
	if err != nil {
		return nil, err
	}

	ch := types.NewChannel(channel)

	// Find last snapshot and apply events after it
	var lastSnapshotIdx = -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type == types.ChannelMsgSnapshot {
			lastSnapshotIdx = i
			ch.Topic = msgs[i].Topic
			ch.Members = make([]string, len(msgs[i].Members))
			copy(ch.Members, msgs[i].Members)
			break
		}
	}

	// Apply events after snapshot
	startIdx := 0
	if lastSnapshotIdx >= 0 {
		startIdx = lastSnapshotIdx + 1
	}

	for i := startIdx; i < len(msgs); i++ {
		msg := msgs[i]
		switch msg.Type {
		case types.ChannelMsgJoin:
			ch.AddMember(msg.User)
		case types.ChannelMsgLeave:
			ch.RemoveMember(msg.User)
		case types.ChannelMsgTopic:
			ch.Topic = msg.Topic
		case types.ChannelMsgMembers:
			ch.Members = make([]string, len(msg.Members))
			copy(ch.Members, msg.Members)
		}
	}

	return ch, nil
}

// AppendUserMsg appends a message to a user's mailbox.
func (s *FileStore) AppendUserMsg(user string, msg *types.UserMessage) error {
	s.locks.Lock("@" + user)
	defer s.locks.Unlock("@" + user)

	path := s.inboxPath("@" + user)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open user file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	_, err = f.Write(append(data, '\n'))
	return err
}

// AppendUserMsgIfNotExists appends a message only if it doesn't already exist.
func (s *FileStore) AppendUserMsgIfNotExists(user string, msg *types.UserMessage) (bool, error) {
	// Check if message already exists
	if msg.ID != "" && s.HasMessage("@"+user, msg.ID) {
		return false, nil
	}

	err := s.AppendUserMsg(user, msg)
	if err != nil {
		return false, err
	}
	return true, nil
}

// ReadUserMsgs reads messages from a user's mailbox.
func (s *FileStore) ReadUserMsgs(user string, opts types.ReadOptions) ([]*types.UserMessage, error) {
	s.locks.RLock("@" + user)
	defer s.locks.RUnlock("@" + user)

	path := s.inboxPath("@" + user)
	messages, err := readFileLines(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []*types.UserMessage
	for _, line := range messages {
		var msg types.UserMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}

		// Apply filters
		if !s.matchesUserFilters(&msg, opts) {
			continue
		}

		// Apply cursor filter
		if opts.AfterCursor != "" && msg.ID <= opts.AfterCursor {
			continue
		}

		result = append(result, &msg)

		// Apply limit
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
	}

	return result, nil
}

// matchesUserFilters checks if a user message matches the filter options.
func (s *FileStore) matchesUserFilters(msg *types.UserMessage, opts types.ReadOptions) bool {
	// Type filter
	if len(opts.Types) > 0 {
		match := false
		for _, t := range opts.Types {
			if string(msg.Type) == t {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Peer filter for DMs
	if opts.Peer != "" && msg.Peer != opts.Peer {
		return false
	}

	// Time filters
	if opts.StartTime != nil && msg.Timestamp.Before(*opts.StartTime) {
		return false
	}
	if opts.EndTime != nil && msg.Timestamp.After(*opts.EndTime) {
		return false
	}

	return true
}

// GetUserMsg gets a specific message from a user's mailbox.
func (s *FileStore) GetUserMsg(user, msgID string) (*types.UserMessage, error) {
	msgs, err := s.ReadUserMsgs(user, types.ReadOptions{})
	if err != nil {
		return nil, err
	}

	for _, msg := range msgs {
		if msg.ID == msgID {
			return msg, nil
		}
	}

	return nil, nil
}

// RebuildUserState rebuilds the user state from storage.
func (s *FileStore) RebuildUserState(user string) (*types.UserState, error) {
	msgs, err := s.ReadUserMsgs(user, types.ReadOptions{})
	if err != nil {
		return nil, err
	}

	state := &types.UserState{
		Subscriptions: make(map[string]*types.Subscription),
		Cursors:       make(map[string]string), // Legacy compatibility
	}

	// Find last snapshot
	var lastSnapshot *types.UserMessage
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type == types.UserMsgSnapshot && msgs[i].State != nil {
			lastSnapshot = msgs[i]
			break
		}
	}

	// Apply snapshot
	if lastSnapshot != nil && lastSnapshot.State != nil {
		// New format: Subscriptions
		if lastSnapshot.State.Subscriptions != nil {
			for inbox, sub := range lastSnapshot.State.Subscriptions {
				state.Subscriptions[inbox] = &types.Subscription{
					SubCursor:  sub.SubCursor,
					ReadCursor: sub.ReadCursor,
				}
			}
		}
		// Legacy format: Cursors (convert to Subscriptions)
		if lastSnapshot.State.Cursors != nil {
			for k, v := range lastSnapshot.State.Cursors {
				if _, ok := state.Subscriptions[k]; !ok {
					state.Subscriptions[k] = &types.Subscription{
						SubCursor:  v,
						ReadCursor: v,
					}
				}
				state.Cursors[k] = v
			}
		}
	}

	// Apply events after snapshot
	startIdx := 0
	if lastSnapshot != nil {
		for i, msg := range msgs {
			if msg == lastSnapshot {
				startIdx = i + 1
				break
			}
		}
	}

	for i := startIdx; i < len(msgs); i++ {
		msg := msgs[i]
		switch msg.Type {
		case types.UserMsgSub:
			if msg.Inbox != "" {
				cursor := msg.Cursor
				if cursor == "" {
					cursor = "0" // Subscribe at beginning if no cursor
				}
				state.Subscriptions[msg.Inbox] = &types.Subscription{
					SubCursor:  cursor,
					ReadCursor: cursor,
				}
				state.Cursors[msg.Inbox] = cursor
				s.debugLog("applied sub event", "user", user, "inbox", msg.Inbox, "cursor", cursor)
			}
		case types.UserMsgMark:
			if msg.Inbox != "" && msg.Cursor != "" {
				if sub, ok := state.Subscriptions[msg.Inbox]; ok {
					sub.ReadCursor = msg.Cursor
				} else {
					// Auto-subscribe if marking without explicit sub
					state.Subscriptions[msg.Inbox] = &types.Subscription{
						SubCursor:  msg.Cursor,
						ReadCursor: msg.Cursor,
					}
				}
				state.Cursors[msg.Inbox] = msg.Cursor
				s.debugLog("applied mark event", "user", user, "inbox", msg.Inbox, "cursor", msg.Cursor)
			}
		case types.UserMsgUnsub:
			delete(state.Subscriptions, msg.Inbox)
			delete(state.Cursors, msg.Inbox)
			s.debugLog("applied unsub event", "user", user, "inbox", msg.Inbox)
		}
	}

	return state, nil
}

// HasMessage checks if a message exists in an inbox.
func (s *FileStore) HasMessage(inbox string, msgID string) bool {
	var msgs interface{}
	var err error

	if strings.HasPrefix(inbox, "#") {
		msgs, err = s.ReadChannelMsgs(inbox, types.ReadOptions{})
	} else {
		msgs, err = s.ReadUserMsgs(strings.TrimPrefix(inbox, "@"), types.ReadOptions{})
	}

	if err != nil {
		return false
	}

	switch m := msgs.(type) {
	case []*types.ChannelMessage:
		for _, msg := range m {
			if msg.ID == msgID {
				return true
			}
		}
	case []*types.UserMessage:
		for _, msg := range m {
			if msg.ID == msgID {
				return true
			}
		}
	}

	return false
}

// GetLastSnapshot gets the last snapshot from an inbox.
func (s *FileStore) GetLastSnapshot(inbox string) (time.Time, interface{}, error) {
	var msgs interface{}
	var err error

	if strings.HasPrefix(inbox, "#") {
		msgs, err = s.ReadChannelMsgs(inbox, types.ReadOptions{})
	} else {
		msgs, err = s.ReadUserMsgs(strings.TrimPrefix(inbox, "@"), types.ReadOptions{})
	}

	if err != nil {
		return time.Time{}, nil, err
	}

	switch m := msgs.(type) {
	case []*types.ChannelMessage:
		for i := len(m) - 1; i >= 0; i-- {
			if m[i].Type == types.ChannelMsgSnapshot {
				return m[i].Timestamp, m[i], nil
			}
		}
	case []*types.UserMessage:
		for i := len(m) - 1; i >= 0; i-- {
			if m[i].Type == types.UserMsgSnapshot {
				return m[i].Timestamp, m[i], nil
			}
		}
	}

	return time.Time{}, nil, nil
}

// CountAfter counts messages after a timestamp.
func (s *FileStore) CountAfter(inbox string, afterTs time.Time) int {
	var count int
	var msgs interface{}
	var err error

	if strings.HasPrefix(inbox, "#") {
		msgs, err = s.ReadChannelMsgs(inbox, types.ReadOptions{})
	} else {
		msgs, err = s.ReadUserMsgs(strings.TrimPrefix(inbox, "@"), types.ReadOptions{})
	}

	if err != nil {
		return 0
	}

	switch m := msgs.(type) {
	case []*types.ChannelMessage:
		for _, msg := range m {
			if msg.Timestamp.After(afterTs) {
				count++
			}
		}
	case []*types.UserMessage:
		for _, msg := range m {
			if msg.Timestamp.After(afterTs) {
				count++
			}
		}
	}

	return count
}

// ListChannels lists all channels.
func (s *FileStore) ListChannels() ([]string, error) {
	entries, err := os.ReadDir(s.dataPath)
	if err != nil {
		return nil, err
	}

	var channels []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		name = strings.TrimSuffix(name, ".jsonl")
		name = decodeInboxName(name)
		if strings.HasPrefix(name, "#") {
			channels = append(channels, name)
		}
	}

	return channels, nil
}

// RepairInbox repairs a corrupted inbox.
func (s *FileStore) RepairInbox(inbox string) (int, error) {
	path := s.inboxPath(inbox)
	lines, err := readFileLines(path)
	if err != nil {
		return 0, err
	}

	var validLines []string
	var removed int

	for _, line := range lines {
		var msg interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			removed++
			continue
		}
		validLines = append(validLines, line)
	}

	// Rewrite file with valid lines only
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	for _, line := range validLines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			return 0, err
		}
	}

	return removed, nil
}
