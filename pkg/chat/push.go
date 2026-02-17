package chat

import (
	"log"
	"sync"
	"time"
)

// PushManager manages real-time message pushing to sessions.
type PushManager struct {
	mu        sync.RWMutex
	sessions  map[string][]*ChatSession // channel -> sessions
	retryCnt  int
}

// NewPushManager creates a new push manager.
func NewPushManager() *PushManager {
	return &PushManager{
		sessions: make(map[string][]*ChatSession),
		retryCnt: 3,
	}
}

// Subscribe associates a session with a channel for push notifications.
func (p *PushManager) Subscribe(channel string, sess *ChatSession) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Also update session's internal subscription
	sess.Subscribe(channel)

	sessions := p.sessions[channel]
	for _, s := range sessions {
		if s.ID() == sess.ID() {
			return // Already subscribed
		}
	}
	p.sessions[channel] = append(sessions, sess)
}

// Unsubscribe removes a session from a channel.
func (p *PushManager) Unsubscribe(channel string, sess *ChatSession) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Also update session's internal subscription
	sess.Unsubscribe(channel)

	sessions := p.sessions[channel]
	for i, s := range sessions {
		if s.ID() == sess.ID() {
			p.sessions[channel] = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}
}

// UnsubscribeAll removes a session from all channels.
func (p *PushManager) UnsubscribeAll(sess *ChatSession) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for channel, sessions := range p.sessions {
		for i, s := range sessions {
			if s.ID() == sess.ID() {
				p.sessions[channel] = append(sessions[:i], sessions[i+1:]...)
				break
			}
		}
	}
}

// PushNotification pushes a notification to all sessions subscribed to a channel.
// This is the new lightweight push mechanism - clients receive only notification
// metadata and can pull full message content if desired.
func (p *PushManager) PushNotification(channel string, notif Notification) {
	p.mu.RLock()
	sessions := p.sessions[channel]
	p.mu.RUnlock()

	for _, sess := range sessions {
		go p.pushNotifToSession(sess, notif)
	}
}

// Push pushes a message to all sessions subscribed to a channel.
// This is the legacy full-message push mechanism.
func (p *PushManager) Push(channel string, msg *ChannelMessage) {
	p.mu.RLock()
	sessions := p.sessions[channel]
	p.mu.RUnlock()

	for _, sess := range sessions {
		go p.pushToSession(sess, msg)
	}
}

// PushUserMessage pushes a user message to a specific session.
func (p *PushManager) PushUserMessage(user string, msg *UserMessage, sess *ChatSession) {
	if sess != nil {
		go p.pushUserMsgToSession(sess, msg)
	}
}

// PushMentionNotification pushes a mention notification to a specific session.
func (p *PushManager) PushMentionNotification(channel, msgID, from, content string, sess *ChatSession) {
	if sess == nil {
		return
	}
	notif := Notification{
		Channel: channel,
		MsgID:   msgID,
		From:    from,
		Content: content,
		Mention: true,
		Time:    time.Now(),
	}
	go sess.PushNotification(notif)
}

func (p *PushManager) pushNotifToSession(sess *ChatSession, notif Notification) {
	// Check if session is done before pushing
	select {
	case <-sess.Done():
		// Session is closed, don't push
		return
	default:
		// Session is still active, proceed
		sess.PushNotification(notif)
	}
}

func (p *PushManager) pushToSession(sess *ChatSession, msg *ChannelMessage) {
	// Check if session is done before pushing
	select {
	case <-sess.Done():
		// Session is closed, don't push
		return
	default:
	}

	for i := 0; i < p.retryCnt; i++ {
		if err := sess.PushChannelMessage(msg); err != nil {
			log.Printf("push message to session %s failed (attempt %d): %v", sess.ID(), i+1, err)
			// Check again if session closed during retry
			select {
			case <-sess.Done():
				return
			default:
			}
			continue
		}
		return
	}
	log.Printf("push message to session %s failed after %d attempts, giving up", sess.ID(), p.retryCnt)
}

func (p *PushManager) pushUserMsgToSession(sess *ChatSession, msg *UserMessage) {
	// Check if session is done before pushing
	select {
	case <-sess.Done():
		// Session is closed, don't push
		return
	default:
	}

	for i := 0; i < p.retryCnt; i++ {
		if err := sess.PushUserMessage(msg); err != nil {
			log.Printf("push user message to session %s failed (attempt %d): %v", sess.ID(), i+1, err)
			// Check again if session closed during retry
			select {
			case <-sess.Done():
				return
			default:
			}
			continue
		}
		return
	}
	log.Printf("push user message to session %s failed after %d attempts, giving up", sess.ID(), p.retryCnt)
}

// GetSubscriberCount returns the number of sessions subscribed to a channel.
func (p *PushManager) GetSubscriberCount(channel string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.sessions[channel])
}

// GetSubscribedChannels returns all channels a session is subscribed to.
func (p *PushManager) GetSubscribedChannels(sess *ChatSession) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var channels []string
	for channel, sessions := range p.sessions {
		for _, s := range sessions {
			if s.ID() == sess.ID() {
				channels = append(channels, channel)
				break
			}
		}
	}
	return channels
}
