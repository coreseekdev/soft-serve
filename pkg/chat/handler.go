package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/ssh"
)

// HandleSession handles a chat session.
func (c *Chat) HandleSession(ctx context.Context, sess ssh.Session, username string) error {
	// Create terminal
	pty, _, active := sess.Pty()
	if !active {
		return fmt.Errorf("pty not active")
	}

	width, height := pty.Window.Width, pty.Window.Height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	terminal := NewSSHTerminal(sess, width, height)

	// Rebuild user state
	state, err := c.Store().RebuildUserState(username)
	if err != nil {
		state = &UserState{Cursors: make(map[string]string)}
	}

	// Create chat user
	user := NewChatUser(username)
	user.State = state

	// Create session
	chatSess := NewChatSession(user, c, terminal)
	c.AddSession(chatSess)
	defer c.RemoveSession(chatSess)

	// Welcome message
	chatSess.WriteLine("Entering chat mode. Type /help for commands.")

	// Show unread counts
	c.showUnreadSummary(chatSess)

	// Handle input loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-chatSess.Done():
				return
			case msg := <-chatSess.Messages():
				chatSess.WriteLine(formatUserMessage(msg))
			}
		}
	}()

	for {
		line, err := terminal.ReadLine()
		if err != nil {
			break
		}

		input := ParseInput(line)
		if input == nil {
			continue
		}

		switch input.Type {
		case InputTypeCommand:
			if err := c.handleCommand(chatSess, input); err != nil {
				chatSess.WriteLine("Error: " + err.Error())
			}

		case InputTypeChannel:
			if err := c.sendChannelMessage(chatSess, input.Target, input.Content); err != nil {
				chatSess.WriteLine("Error: " + err.Error())
			}

		case InputTypeMessage:
			if input.Target != "" {
				// Select target
				tt, tv := ParseTarget(input.Target)
				if tt == TargetTypeChannel {
					user.SetCurrentChannel(tv)
				} else {
					user.SetCurrentPeer(tv)
				}
			} else if input.Content != "" {
				// Send to current target
				if err := c.sendToCurrentTarget(chatSess, input.Content); err != nil {
					chatSess.WriteLine("Error: " + err.Error())
				}
			}
		}
	}

	return nil
}

func (c *Chat) handleCommand(sess *ChatSession, input *ParsedInput) error {
	handler, ok := GetCommandHandler(input.Command)
	if !ok {
		return fmt.Errorf("unknown command: %s", input.Command)
	}
	return handler(sess, input.Args)
}

func (c *Chat) sendChannelMessage(sess *ChatSession, channel, content string) error {
	user := sess.User()

	// Validate channel name
	if err := ValidateChannelName(channel); err != nil {
		return err
	}

	// Validate and sanitize message content
	if err := ValidateMessage(content); err != nil {
		return err
	}
	content = SanitizeMessage(content)

	// Ensure channel exists and user is member
	ch, err := c.GetOrCreateChannel(channel)
	if err != nil {
		return err
	}

	// Auto-join if not member
	if !ch.HasMember(user.Name) {
		if err := handleJoin(sess, []string{channel}); err != nil {
			return err
		}
		ch, _ = c.GetChannel(channel)
	}

	// Parse mentions
	mentions := ParseMentions(content, c.GetUserListMap())

	// Create message
	msgID := c.GenerateMessageID()
	msg := NewChannelMessage(ChannelMsgMessage)
	msg.ID = msgID
	msg.From = user.Name
	msg.Content = content
	msg.Mentions = mentions

	// Write to channel
	if err := c.Store().AppendChannelMsg(channel, msg); err != nil {
		return err
	}

	// Write to sender's mailbox
	userMsg := NewUserMessage(UserMsgChannel)
	userMsg.ID = msgID
	userMsg.Direction = "out"
	userMsg.Peer = channel
	userMsg.Content = content
	userMsg.Mentions = mentions
	if err := c.Store().AppendUserMsg(user.Name, userMsg); err != nil {
		return err
	}

	// Send mention notifications (store + push to online users)
	for _, mentioned := range mentions {
		// Store mention notification
		mentionMsg := NewUserMessage(UserMsgMention)
		mentionMsg.ID = msgID
		mentionMsg.Inbox = channel
		mentionMsg.From = user.Name
		mentionMsg.Content = content
		c.Store().AppendUserMsg(mentioned, mentionMsg)

		// Push mention notification if user is online
		if onlineSess, ok := c.GetSession(mentioned); ok {
			c.pushMgr.PushMentionNotification(channel, msgID, user.Name, content, onlineSess)
		}
	}

	// Push notification to all online subscribers
	notif := Notification{
		Channel: channel,
		MsgID:   msgID,
		From:    user.Name,
		Mention: false, // Will be set to true for mentioned users
		Time:    msg.Timestamp,
	}
	c.pushMgr.PushNotification(channel, notif)

	return nil
}

func (c *Chat) sendToCurrentTarget(sess *ChatSession, content string) error {
	user := sess.User()

	if user.CurrentType == TargetTypeChannel {
		if user.CurrentCh == "" {
			return fmt.Errorf("no channel selected")
		}
		return c.sendChannelMessage(sess, user.CurrentCh, content)
	}

	if user.CurrentPeer != "" {
		return c.sendDirectMessage(sess, user.CurrentPeer, content)
	}

	return fmt.Errorf("no target selected")
}

func (c *Chat) sendDirectMessage(sess *ChatSession, peer, content string) error {
	user := sess.User()

	// Validate username
	if err := ValidateUsername(peer); err != nil {
		return err
	}

	// Validate and sanitize message content
	if err := ValidateMessage(content); err != nil {
		return err
	}
	content = SanitizeMessage(content)

	if !c.IsValidUser(peer) {
		return fmt.Errorf("user not found: %s", peer)
	}

	// Create message
	msgID := c.GenerateMessageID()
	msg := NewUserMessage(UserMsgDM)
	msg.ID = msgID
	msg.Direction = "out"
	msg.Peer = "@" + peer
	msg.Content = content

	// Write to sender's mailbox
	if err := c.Store().AppendUserMsg(user.Name, msg); err != nil {
		return err
	}

	// Write to receiver's mailbox
	recvMsg := NewUserMessage(UserMsgDM)
	recvMsg.ID = msgID
	recvMsg.Direction = "in"
	recvMsg.From = user.Name
	recvMsg.Peer = "@" + user.Name
	recvMsg.Content = content
	c.Store().AppendUserMsgIfNotExists(peer, recvMsg)

	sess.WriteLine("[DM -> @" + peer + "] " + content)
	return nil
}

func (c *Chat) showUnreadSummary(sess *ChatSession) {
	user := sess.User()
	var totalUnread, totalMentions int

	for inbox := range user.State.Cursors {
		cursor := user.GetCursor(inbox)
		if strings.HasPrefix(inbox, "#") {
			count := c.countUnreadChannel(inbox, cursor)
			if count > 0 {
				totalUnread += count
			}
		}
	}

	// Count mentions
	mentions, _ := c.Store().ReadUserMsgs(user.Name, ReadOptions{
		Types: []string{"mention"},
		Limit: 1000,
	})
	totalMentions = len(mentions)

	if totalUnread > 0 || totalMentions > 0 {
		msg := fmt.Sprintf("You have %d unread messages", totalUnread)
		if totalMentions > 0 {
			msg += fmt.Sprintf(", %d mentions", totalMentions)
		}
		sess.WriteLine(msg)
	}
}

func (c *Chat) countUnreadChannel(channel, cursor string) int {
	msgs, _ := c.Store().ReadChannelMsgs(channel, ReadOptions{
		AfterCursor: cursor,
		Types:       []string{"msg"},
		Limit:       10000,
	})
	return len(msgs)
}

// SSHTerminal wraps an SSH session as a Terminal.
type SSHTerminal struct {
	sess   ssh.Session
	width  int
	height int
}

// NewSSHTerminal creates a new SSH terminal.
func NewSSHTerminal(sess ssh.Session, width, height int) *SSHTerminal {
	return &SSHTerminal{
		sess:   sess,
		width:  width,
		height: height,
	}
}

// Write writes bytes to the terminal.
func (t *SSHTerminal) Write(b []byte) (int, error) {
	return t.sess.Write(b)
}

// WriteString writes a string to the terminal.
func (t *SSHTerminal) WriteString(s string) (int, error) {
	return t.sess.Write([]byte(s))
}

// ReadLine reads a line from the terminal.
func (t *SSHTerminal) ReadLine() (string, error) {
	// Simple line reading implementation
	buf := make([]byte, 1)
	var line []byte
	for {
		n, err := t.sess.Read(buf)
		if err != nil || n == 0 {
			return "", err
		}
		if buf[0] == '\n' || buf[0] == '\r' {
			return string(line), nil
		}
		line = append(line, buf[0])
	}
}

// Close closes the terminal.
func (t *SSHTerminal) Close() error {
	return t.sess.Close()
}

// SetPrompt sets the terminal prompt.
func (t *SSHTerminal) SetPrompt(prompt string) {
	// Not implemented for simple terminal
}

// Resize resizes the terminal.
func (t *SSHTerminal) Resize(width, height int) {
	t.width = width
	t.height = height
}
