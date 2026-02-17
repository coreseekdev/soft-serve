package chat

import (
	"fmt"
	"regexp"
	"strings"
)

// InputType represents the type of parsed input.
type InputType int

const (
	InputTypeCommand InputType = iota // /xxx command
	InputTypeChannel                  // #channel xxx channel message
	InputTypeMessage                  // xxx message to current target
)

// ParsedInput represents a parsed user input.
type ParsedInput struct {
	Type    InputType
	Target  string   // Target: #channel (for channel messages)
	Content string   // Message content
	Command string   // Command name (for commands)
	Args    []string // Command arguments
}

// ParseInput parses user input.
func ParseInput(input string) *ParsedInput {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Command: /xxx
	if strings.HasPrefix(input, "/") {
		parts := strings.Fields(input[1:])
		if len(parts) == 0 {
			return nil
		}
		return &ParsedInput{
			Type:    InputTypeCommand,
			Command: strings.ToLower(parts[0]),
			Args:    parts[1:],
		}
	}

	// Channel message: #channel xxx
	if strings.HasPrefix(input, "#") {
		parts := strings.SplitN(input[1:], " ", 2)
		if len(parts) == 2 && parts[1] != "" {
			return &ParsedInput{
				Type:    InputTypeChannel,
				Target:  "#" + parts[0],
				Content: parts[1],
			}
		}
		// Just #channel with no message - could be select
		return &ParsedInput{
			Type:   InputTypeMessage,
			Target: "#" + parts[0],
		}
	}

	// Current target message
	return &ParsedInput{
		Type:    InputTypeMessage,
		Content: input,
	}
}

// CommandHandler handles a command.
type CommandHandler func(sess *ChatSession, args []string) error

// CommandHandlers maps command names to handlers.
var commandHandlers = map[string]CommandHandler{
	"join":     handleJoin,
	"leave":    handleLeave,
	"select":   handleSelect,
	"dm":       handleDM,
	"mark":     handleMark,
	"history":  handleHistory,
	"mentions": handleMentions,
	"topic":    handleTopic,
	"channels": handleChannels,
	"subs":     handleSubs,
	"who":      handleWho,
	"names":    handleNames,
	"whois":    handleWhois,
	"help":     handleHelp,
	"exit":     handleExit,
	"quit":     handleExit,
}

// mentionRegex matches @mentions in messages.
var mentionRegex = regexp.MustCompile(`(?:^|[\s,.;:!?()\[\]{}])(@|＠)([\p{L}\p{N}_]+)`)

// ParseMentions parses @mentions from message content.
func ParseMentions(content string, validUsers map[string]bool) []string {
	var mentions []string
	seen := make(map[string]bool)

	matches := mentionRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		name := strings.ToLower(match[2])
		// Maximum matching: try to match longer usernames first
		for {
			if validUsers[name] && !seen[name] {
				mentions = append(mentions, name)
				seen[name] = true
				break
			}
			// Try shorter match
			if len(name) <= 1 {
				break
			}
			name = name[:len(name)-1]
		}
	}

	return mentions
}

// GetCommandHandler returns the handler for a command.
func GetCommandHandler(cmd string) (CommandHandler, bool) {
	h, ok := commandHandlers[cmd]
	return h, ok
}

// Command implementations

func handleJoin(sess *ChatSession, args []string) error {
	if len(args) == 0 {
		return sess.WriteLine("Usage: /join #channel")
	}

	channel := args[0]
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	// Get or create channel
	ch, err := sess.chat.GetOrCreateChannel(channel)
	if err != nil {
		return sess.WriteLine("Error joining channel: " + err.Error())
	}

	// Add user to channel
	user := sess.User()
	if !ch.HasMember(user.Name) {
		ch.AddMember(user.Name)

		// Write join event
		msg := NewChannelMessage(ChannelMsgJoin)
		msg.User = user.Name
		msg.Members = ch.Members
		if err := sess.chat.Store().AppendChannelMsg(channel, msg); err != nil {
			return sess.WriteLine("Error recording join: " + err.Error())
		}
	}

	// Get current last message ID for cursor
	lastID, _ := sess.chat.Store().GetLastChannelMsgID(channel)

	// Subscribe user using new subscription model
	user.State.SetSubscription(channel, lastID)

	// Write subscription
	subMsg := NewUserMessage(UserMsgSub)
	subMsg.Inbox = channel
	subMsg.Cursor = lastID
	if err := sess.chat.Store().AppendUserMsg(user.Name, subMsg); err != nil {
		return sess.WriteLine("Error subscribing: " + err.Error())
	}

	// Set as current target
	user.SetCurrentChannel(channel)

	// Subscribe session to push
	sess.chat.pushMgr.Subscribe(channel, sess)

	return sess.WriteLine("Joined " + channel + " (" + fmt.Sprintf("%d", ch.MemberCount()) + " members)")
}

func handleLeave(sess *ChatSession, args []string) error {
	user := sess.User()
	channel := user.CurrentCh

	if len(args) > 0 {
		channel = args[0]
		if !strings.HasPrefix(channel, "#") {
			channel = "#" + channel
		}
	}

	if channel == "" {
		return sess.WriteLine("Usage: /leave #channel")
	}

	ch, ok := sess.chat.GetChannel(channel)
	if !ok {
		return sess.WriteLine("Channel not found: " + channel)
	}

	// Remove user from channel
	if ch.RemoveMember(user.Name) {
		// Write leave event
		msg := NewChannelMessage(ChannelMsgLeave)
		msg.User = user.Name
		msg.Members = ch.Members
		if err := sess.chat.Store().AppendChannelMsg(channel, msg); err != nil {
			return sess.WriteLine("Error recording leave: " + err.Error())
		}
	}

	// Unsubscribe user using new subscription model
	user.State.RemoveSubscription(channel)

	// Write unsubscription
	unsubMsg := NewUserMessage(UserMsgUnsub)
	unsubMsg.Inbox = channel
	if err := sess.chat.Store().AppendUserMsg(user.Name, unsubMsg); err != nil {
		return sess.WriteLine("Error unsubscribing: " + err.Error())
	}

	// Unsubscribe session from push
	sess.chat.pushMgr.Unsubscribe(channel, sess)

	return sess.WriteLine("Left " + channel)
}

func handleSelect(sess *ChatSession, args []string) error {
	if len(args) == 0 {
		return sess.WriteLine("Usage: /select #channel or /select @user")
	}

	target := args[0]
	user := sess.User()

	if strings.HasPrefix(target, "#") {
		// Select channel
		ch, ok := sess.chat.GetChannel(target)
		if !ok {
			return sess.WriteLine("Channel not found: " + target)
		}
		user.SetCurrentChannel(target)
		return sess.WriteLine("Now in " + target + " (" + fmt.Sprintf("%d", ch.MemberCount()) + " members)")
	}

	if strings.HasPrefix(target, "@") {
		// Select private chat
		peer := strings.TrimPrefix(target, "@")
		if !sess.chat.IsValidUser(peer) {
			return sess.WriteLine("User not found: " + peer)
		}
		user.SetCurrentPeer(peer)
		return sess.WriteLine("Now talking with " + target)
	}

	return sess.WriteLine("Invalid target: " + target)
}

func handleDM(sess *ChatSession, args []string) error {
	if len(args) < 2 {
		return sess.WriteLine("Usage: /dm @user message")
	}

	peer := args[0]
	if !strings.HasPrefix(peer, "@") {
		peer = "@" + peer
	}
	peerName := strings.TrimPrefix(peer, "@")

	if !sess.chat.IsValidUser(peerName) {
		return sess.WriteLine("User not found: " + peerName)
	}

	content := strings.Join(args[1:], " ")
	user := sess.User()

	// Create message
	msgID := sess.chat.GenerateMessageID()
	msg := NewUserMessage(UserMsgDM)
	msg.ID = msgID
	msg.Direction = "out"
	msg.Peer = peer
	msg.Content = content

	// Write to sender's mailbox
	if err := sess.chat.Store().AppendUserMsg(user.Name, msg); err != nil {
		return sess.WriteLine("Error sending message: " + err.Error())
	}

	// Write to receiver's mailbox
	recvMsg := NewUserMessage(UserMsgDM)
	recvMsg.ID = msgID
	recvMsg.Direction = "in"
	recvMsg.From = user.Name
	recvMsg.Peer = "@" + user.Name
	recvMsg.Content = content

	if _, err := sess.chat.Store().AppendUserMsgIfNotExists(peerName, recvMsg); err != nil {
		return sess.WriteLine("Error delivering message: " + err.Error())
	}

	return sess.WriteLine("[DM -> " + peer + "] " + content)
}

func handleMark(sess *ChatSession, args []string) error {
	user := sess.User()
	var inbox string

	if len(args) > 0 {
		inbox = args[0]
	} else {
		if user.CurrentType == TargetTypeChannel {
			inbox = user.CurrentCh
		} else {
			inbox = "@" + user.CurrentPeer
		}
	}

	if inbox == "" {
		return sess.WriteLine("Usage: /mark [#channel|@user]")
	}

	var lastID string
	var err error

	if strings.HasPrefix(inbox, "#") {
		lastID, err = sess.chat.Store().GetLastChannelMsgID(inbox)
	} else {
		// For DM, get last DM message ID
		msgs, e := sess.chat.Store().ReadUserMsgs(user.Name, ReadOptions{
			Types: []string{"dm"},
			Peer:  inbox,
			Limit: 1,
		})
		if e == nil && len(msgs) > 0 {
			lastID = msgs[0].ID
		}
	}

	if err != nil {
		return sess.WriteLine("Error getting last message: " + err.Error())
	}

	// Update read cursor using new subscription model
	user.State.SetReadCursor(inbox, lastID)

	// Write mark message
	markMsg := NewUserMessage(UserMsgMark)
	markMsg.Inbox = inbox
	markMsg.Cursor = lastID
	if err := sess.chat.Store().AppendUserMsg(user.Name, markMsg); err != nil {
		return sess.WriteLine("Error marking read: " + err.Error())
	}

	return sess.WriteLine("Marked " + inbox + " as read")
}

func handleHistory(sess *ChatSession, args []string) error {
	user := sess.User()
	var inbox string
	showAll := false

	// Parse args
	for _, arg := range args {
		if arg == "--all" {
			showAll = true
		} else if strings.HasPrefix(arg, "#") || strings.HasPrefix(arg, "@") {
			inbox = arg
		}
	}

	if inbox == "" {
		if user.CurrentType == TargetTypeChannel {
			inbox = user.CurrentCh
		} else {
			inbox = "@" + user.CurrentPeer
		}
	}

	if inbox == "" {
		return sess.WriteLine("Usage: /history [#channel|@user] [--all]")
	}

	var cursor string
	if !showAll {
		cursor = user.GetCursor(inbox)
	}

	if strings.HasPrefix(inbox, "#") {
		// Channel history
		opts := ReadOptions{
			AfterCursor: cursor,
			Types:       []string{"msg"},
			Limit:       100,
		}
		msgs, err := sess.chat.Store().ReadChannelMsgs(inbox, opts)
		if err != nil {
			return sess.WriteLine("Error reading history: " + err.Error())
		}

		sess.WriteLine("History for " + inbox + ":")
		for _, msg := range msgs {
			sess.WriteLine(formatChannelMessage(msg))
		}
		if len(msgs) == 0 {
			sess.WriteLine("No messages")
		}
	} else {
		// DM history
		opts := ReadOptions{
			AfterCursor: cursor,
			Types:       []string{"dm"},
			Peer:        inbox,
			Limit:       100,
		}
		msgs, err := sess.chat.Store().ReadUserMsgs(user.Name, opts)
		if err != nil {
			return sess.WriteLine("Error reading history: " + err.Error())
		}

		sess.WriteLine("History with " + inbox + ":")
		for _, msg := range msgs {
			sess.WriteLine(formatUserMessage(msg))
		}
		if len(msgs) == 0 {
			sess.WriteLine("No messages")
		}
	}

	return nil
}

func handleMentions(sess *ChatSession, args []string) error {
	user := sess.User()

	opts := ReadOptions{
		Types: []string{"mention"},
		Limit: 50,
	}
	msgs, err := sess.chat.Store().ReadUserMsgs(user.Name, opts)
	if err != nil {
		return sess.WriteLine("Error reading mentions: " + err.Error())
	}

	sess.WriteLine("Your mentions:")
	for _, msg := range msgs {
		sess.WriteLine(formatUserMessage(msg))
	}
	if len(msgs) == 0 {
		sess.WriteLine("No mentions")
	}

	return nil
}

func handleTopic(sess *ChatSession, args []string) error {
	user := sess.User()
	channel := user.CurrentCh

	if len(args) > 0 && strings.HasPrefix(args[0], "#") {
		channel = args[0]
		args = args[1:]
	}

	if channel == "" {
		return sess.WriteLine("Usage: /topic [#channel] [topic]")
	}

	ch, ok := sess.chat.GetChannel(channel)
	if !ok {
		return sess.WriteLine("Channel not found: " + channel)
	}

	if len(args) == 0 {
		// Show topic
		if ch.Topic == "" {
			return sess.WriteLine("No topic set for " + channel)
		}
		return sess.WriteLine("Topic for " + channel + ": " + ch.Topic)
	}

	// Set topic
	topic := strings.Join(args, " ")
	ch.Topic = topic

	// Write topic event
	msg := NewChannelMessage(ChannelMsgTopic)
	msg.Topic = topic
	msg.User = user.Name
	if err := sess.chat.Store().AppendChannelMsg(channel, msg); err != nil {
		return sess.WriteLine("Error setting topic: " + err.Error())
	}

	return sess.WriteLine("Topic set for " + channel)
}

func handleChannels(sess *ChatSession, args []string) error {
	channels, err := sess.chat.Store().ListChannels()
	if err != nil {
		return sess.WriteLine("Error listing channels: " + err.Error())
	}

	sess.WriteLine("Channels:")
	for _, name := range channels {
		ch, _ := sess.chat.GetChannel(name)
		if ch != nil {
			topic := ch.Topic
			if topic == "" {
				topic = "(no topic)"
			}
			sess.WriteLine(name + " (" + fmt.Sprintf("%d", ch.MemberCount()) + " users) - " + topic)
		}
	}
	if len(channels) == 0 {
		sess.WriteLine("No channels")
	}

	return nil
}

func handleSubs(sess *ChatSession, args []string) error {
	user := sess.User()

	if user.State == nil || user.State.Subscriptions == nil {
		return sess.WriteLine("No subscriptions")
	}

	sess.WriteLine("Your subscriptions:")
	for inbox, sub := range user.State.Subscriptions {
		// Count unread messages
		var unread int
		if strings.HasPrefix(inbox, "#") {
			msgs, _ := sess.chat.Store().ReadChannelMsgs(inbox, ReadOptions{
				AfterCursor: sub.ReadCursor,
				Types:       []string{"msg"},
				Limit:       10000,
			})
			unread = len(msgs)
		}
		if unread > 0 {
			sess.WriteLine("  " + inbox + " (unread: " + fmt.Sprintf("%d", unread) + ")")
		} else {
			sess.WriteLine("  " + inbox)
		}
	}
	if len(user.State.Subscriptions) == 0 {
		sess.WriteLine("  (none)")
	}

	return nil
}

func handleWho(sess *ChatSession, args []string) error {
	user := sess.User()
	channel := user.CurrentCh

	if len(args) > 0 {
		channel = args[0]
	}

	if channel == "" {
		return sess.WriteLine("Usage: /who [#channel]")
	}

	ch, ok := sess.chat.GetChannel(channel)
	if !ok {
		return sess.WriteLine("Channel not found: " + channel)
	}

	sess.WriteLine("Users in " + channel + ": " + strings.Join(ch.Members, ", "))
	return nil
}

func handleNames(sess *ChatSession, args []string) error {
	if len(args) == 0 {
		return sess.WriteLine("Usage: /names #channel")
	}

	channel := args[0]
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	ch, ok := sess.chat.GetChannel(channel)
	if !ok {
		return sess.WriteLine("Channel not found: " + channel)
	}

	sess.WriteLine("Users in " + channel + ": " + strings.Join(ch.Members, ", "))
	return nil
}

func handleWhois(sess *ChatSession, args []string) error {
	if len(args) == 0 {
		return sess.WriteLine("Usage: /whois @user")
	}

	username := args[0]
	if strings.HasPrefix(username, "@") {
		username = strings.TrimPrefix(username, "@")
	}

	if !sess.chat.IsValidUser(username) {
		return sess.WriteLine("User not found: " + username)
	}

	// Show user info
	sess.WriteLine("User: " + username)

	// Check if online
	if sess.chat.IsUserOnline(username) {
		sess.WriteLine("Status: online")
	} else {
		sess.WriteLine("Status: offline")
	}

	return nil
}

func handleHelp(sess *ChatSession, args []string) error {
	help := `Chat Commands:
  /join #channel         Join or create a channel
  /leave [#channel]      Leave a channel
  /select #channel|@user Switch current target
  /dm @user message      Send private message
  /mark [#channel|@user] Mark as read
  /history [--all]       Show message history
  /mentions              Show your mentions
  /topic [#channel] [topic] View/set channel topic
  /channels              List all channels
  /subs                  List your subscriptions
  /who [#channel]        List users in channel
  /names #channel        List users in channel
  /whois @user           Show user info
  /help                  Show this help
  /exit                  Exit chat mode

Message shortcuts:
  #channel message       Send message to channel
  message               Send message to current target`
	return sess.WriteLine(help)
}

func handleExit(sess *ChatSession, args []string) error {
	return sess.WriteLine("Goodbye!")
}
