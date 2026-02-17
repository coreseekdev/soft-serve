package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/log/v2"
	"github.com/charmbracelet/soft-serve/pkg/chat"
	"github.com/charmbracelet/soft-serve/pkg/chat/types"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/charmbracelet/ssh"
	"github.com/spf13/cobra"
)

// chatInputFile is the input file for chat commands
var chatInputFile string

// chatWatchMode enables watch mode
var chatWatchMode bool

// ChatCommand returns the chat command.
func ChatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Chat commands",
	}

	cmd.AddCommand(
		ChatSendCommand(),
		ChatSubCommand(),
		ChatUnsubCommand(),
		ChatMarkCommand(),
		ChatStatusCommand(),
		ChatPullCommand(),
		ChatListCommand(),
		ChatWatchCommand(),
	)

	return cmd
}

// ChatSendCommand sends a message to a channel.
func ChatSendCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send [channel] [message]",
		Short: "Send a message to a channel",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			channel := args[0]
			message := strings.Join(args[1:], " ")

			if !strings.HasPrefix(channel, "#") {
				channel = "#" + channel
			}

			return sendChatMessage(cmd, ctx, channel, message)
		},
	}

	return cmd
}

// ChatSubCommand subscribes to a channel.
func ChatSubCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sub [channel]",
		Short: "Subscribe to a channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			channel := args[0]

			if !strings.HasPrefix(channel, "#") {
				channel = "#" + channel
			}

			return subscribeChannel(cmd, ctx, channel)
		},
	}

	return cmd
}

// ChatUnsubCommand unsubscribes from a channel.
func ChatUnsubCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unsub [channel]",
		Short: "Unsubscribe from a channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			channel := args[0]

			if !strings.HasPrefix(channel, "#") {
				channel = "#" + channel
			}

			return unsubscribeChannel(cmd, ctx, channel)
		},
	}

	return cmd
}

// ChatMarkCommand marks messages as read.
func ChatMarkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mark [inbox]",
		Short: "Mark messages in an inbox as read",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			inbox := ""
			if len(args) > 0 {
				inbox = args[0]
			}
			return markAsRead(cmd, ctx, inbox)
		},
	}

	return cmd
}

// ChatStatusCommand shows unread message summary.
func ChatStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show unread message summary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return showStatus(cmd, ctx)
		},
	}

	return cmd
}

// ChatPullCommand pulls unread messages.
func ChatPullCommand() *cobra.Command {
	var mentionsOnly bool
	var limit int

	cmd := &cobra.Command{
		Use:   "pull [inbox]",
		Short: "Pull unread messages from an inbox",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			inbox := ""
			if len(args) > 0 {
				inbox = args[0]
			}
			return pullMessages(cmd, ctx, inbox, mentionsOnly, limit)
		},
	}

	cmd.Flags().BoolVar(&mentionsOnly, "mentions", false, "Only pull mentions")
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "Maximum number of messages to pull")

	return cmd
}

// ChatListCommand lists channels or messages.
func ChatListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls [channel]",
		Short: "List channels or recent messages in a channel",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if len(args) == 0 {
				return listChannels(ctx, cmd)
			}

			channel := args[0]
			if !strings.HasPrefix(channel, "#") {
				channel = "#" + channel
			}
			return listMessages(ctx, cmd, channel)
		},
	}

	return cmd
}

// ChatWatchCommand watches for new messages.
func ChatWatchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch [channel]",
		Short: "Watch for new messages in a channel (or DMs if no channel specified)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			channel := ""
			if len(args) > 0 {
				channel = args[0]
				if !strings.HasPrefix(channel, "#") && !strings.HasPrefix(channel, "@") {
					channel = "#" + channel
				}
			}

			return watchMessages(ctx, cmd, channel)
		},
	}

	cmd.Flags().BoolVarP(&chatWatchMode, "watch", "w", false, "Keep watching for new messages")

	return cmd
}

// ChatInteractiveCommand is the main interactive chat command
func ChatInteractiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Interactive chat mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// If -i is set without value, use stdin
			if cmd.Flags().Changed("input") && chatInputFile == "" {
				return processInputReader(ctx, cmd, os.Stdin, "stdin")
			}

			// If -i has a filename, read from file
			if chatInputFile != "" {
				return processInputFile(ctx, cmd, chatInputFile)
			}

			return interactiveChat(ctx, cmd)
		},
	}

	cmd.Flags().StringVarP(&chatInputFile, "input", "i", "", "Input file with commands (omit value to read from stdin)")
	cmd.Flags().BoolVarP(&chatWatchMode, "watch", "w", false, "Watch mode - keep connection alive for incoming messages")

	return cmd
}

func getChatAndUser(ctx context.Context) (*chat.Chat, proto.User, error) {
	chatInstance := chat.FromContext(ctx)
	if chatInstance == nil {
		return nil, nil, fmt.Errorf("chat is not enabled")
	}

	user := proto.UserFromContext(ctx)
	if user == nil {
		return nil, nil, fmt.Errorf("not authenticated")
	}

	return chatInstance, user, nil
}

func sendChatMessage(cmd *cobra.Command, ctx context.Context, channel, message string) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()
	msgID := chatInstance.GenerateMessageID()

	err = store.AppendChannelMsg(channel, &types.ChannelMessage{
		ID:        msgID,
		Type:      types.ChannelMsgMessage,
		From:      user.Username(),
		Content:   message,
		Timestamp: time.Now(),
	})

	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	cmd.Printf("Message sent to %s\n", channel)
	return nil
}

func subscribeChannel(cmd *cobra.Command, ctx context.Context, channel string) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()

	// Get current last message ID for cursor
	lastID, err := store.GetLastChannelMsgID(channel)
	if err != nil {
		return fmt.Errorf("failed to get channel cursor: %w", err)
	}

	// Subscribe to channel
	err = store.AppendUserMsg(user.Username(), &types.UserMessage{
		Type:      types.UserMsgSub,
		Inbox:     channel,
		Cursor:    lastID,
		Timestamp: time.Now(),
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	cmd.Printf("Subscribed to %s (cursor: %s)\n", channel, lastID)
	return nil
}

func unsubscribeChannel(cmd *cobra.Command, ctx context.Context, channel string) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()

	// Unsubscribe from channel
	err = store.AppendUserMsg(user.Username(), &types.UserMessage{
		Type:      types.UserMsgUnsub,
		Inbox:     channel,
		Timestamp: time.Now(),
	})

	if err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	cmd.Printf("Unsubscribed from %s\n", channel)
	return nil
}

func markAsRead(cmd *cobra.Command, ctx context.Context, inbox string) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()
	username := user.Username()

	// If no inbox specified, show usage
	if inbox == "" {
		return fmt.Errorf("usage: chat mark <inbox>\nUse 'chat status' to see your subscriptions")
	}

	var lastID string
	if strings.HasPrefix(inbox, "#") {
		lastID, err = store.GetLastChannelMsgID(inbox)
		if err != nil {
			return fmt.Errorf("failed to get cursor: %w", err)
		}
	} else {
		// For DMs, get the last DM message
		msgs, err := store.ReadUserMsgs(username, types.ReadOptions{
			Types: []string{"dm"},
			Peer:  inbox,
			Limit: 1,
		})
		if err == nil && len(msgs) > 0 {
			lastID = msgs[0].ID
		}
	}

	if lastID == "" {
		cmd.Printf("No messages in %s\n", inbox)
		return nil
	}

	// Write mark message
	err = store.AppendUserMsg(username, &types.UserMessage{
		Type:      types.UserMsgMark,
		Inbox:     inbox,
		Cursor:    lastID,
		Timestamp: time.Now(),
	})

	if err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	cmd.Printf("Marked %s as read (cursor: %s)\n", inbox, lastID)
	return nil
}

func showStatus(cmd *cobra.Command, ctx context.Context) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()
	username := user.Username()

	// Rebuild user state to get subscriptions
	state, err := store.RebuildUserState(username)
	if err != nil {
		return fmt.Errorf("failed to get user state: %w", err)
	}

	if state == nil || len(state.Subscriptions) == 0 {
		cmd.Println("No subscriptions")
		return nil
	}

	cmd.Println("Subscriptions:")
	for inbox, sub := range state.Subscriptions {
		var unread int
		if strings.HasPrefix(inbox, "#") {
			msgs, _ := store.ReadChannelMsgs(inbox, types.ReadOptions{
				AfterCursor: sub.ReadCursor,
				Types:       []string{"msg"},
				Limit:       10000,
			})
			unread = len(msgs)
		}

		if unread > 0 {
			cmd.Printf("  %s: %d unread\n", inbox, unread)
		} else {
			cmd.Printf("  %s: up to date\n", inbox)
		}
	}

	return nil
}

func pullMessages(cmd *cobra.Command, ctx context.Context, inbox string, mentionsOnly bool, limit int) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()
	username := user.Username()

	// Get user state
	state, err := store.RebuildUserState(username)
	if err != nil {
		return fmt.Errorf("failed to get user state: %w", err)
	}

	if inbox != "" {
		// Pull from specific inbox
		return pullFromInbox(cmd, store, username, inbox, state, mentionsOnly, limit)
	}

	// Pull from all subscriptions
	if state == nil || len(state.Subscriptions) == 0 {
		cmd.Println("No subscriptions")
		return nil
	}

	for inbox := range state.Subscriptions {
		if err := pullFromInbox(cmd, store, username, inbox, state, mentionsOnly, limit); err != nil {
			cmd.Printf("Error pulling from %s: %s\n", inbox, err)
		}
	}

	return nil
}

func pullFromInbox(cmd *cobra.Command, store types.ChatStore, username, inbox string, state *types.UserState, mentionsOnly bool, limit int) error {
	var cursor string
	if state != nil && state.Subscriptions[inbox] != nil {
		cursor = state.Subscriptions[inbox].ReadCursor
	}

	if strings.HasPrefix(inbox, "#") {
		opts := types.ReadOptions{
			AfterCursor: cursor,
			Types:       []string{"msg"},
			Limit:       limit,
		}

		msgs, err := store.ReadChannelMsgs(inbox, opts)
		if err != nil {
			return err
		}

		if len(msgs) == 0 {
			return nil
		}

		cmd.Printf("\n%s (%d messages):\n", inbox, len(msgs))
		for _, msg := range msgs {
			if mentionsOnly {
				// Check if user is mentioned
				mentioned := false
				for _, m := range msg.Mentions {
					if m == username {
						mentioned = true
						break
					}
				}
				if !mentioned {
					continue
				}
			}

			ts := msg.Timestamp.Format("2006-01-02 15:04:05")
			mentionStr := ""
			for _, m := range msg.Mentions {
				if m == username {
					mentionStr = " [@you]"
					break
				}
			}
			cmd.Printf("[%s] <%s>%s %s\n", ts, msg.From, mentionStr, msg.Content)
		}
	} else {
		// DMs
		opts := types.ReadOptions{
			AfterCursor: cursor,
			Types:       []string{"dm"},
			Peer:        inbox,
			Limit:       limit,
		}

		msgs, err := store.ReadUserMsgs(username, opts)
		if err != nil {
			return err
		}

		if len(msgs) == 0 {
			return nil
		}

		cmd.Printf("\nDMs with %s (%d messages):\n", inbox, len(msgs))
		for _, msg := range msgs {
			ts := msg.Timestamp.Format("2006-01-02 15:04:05")
			if msg.Direction == "in" {
				cmd.Printf("[%s] <%s> %s\n", ts, msg.From, msg.Content)
			} else {
				cmd.Printf("[%s] -> %s: %s\n", ts, msg.Peer, msg.Content)
			}
		}
	}

	return nil
}

func listChannels(ctx context.Context, cmd *cobra.Command) error {
	chatInstance, _, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()
	channels, err := store.ListChannels()
	if err != nil {
		return fmt.Errorf("failed to list channels: %w", err)
	}

	if len(channels) == 0 {
		cmd.Println("No channels found")
		return nil
	}

	cmd.Println("Channels:")
	for _, ch := range channels {
		cmd.Printf("  %s\n", ch)
	}

	return nil
}

func listMessages(ctx context.Context, cmd *cobra.Command, channel string) error {
	chatInstance, _, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()
	msgs, err := store.ReadChannelMsgs(channel, types.ReadOptions{
		Limit: 50,
		Types: []string{"msg"},
	})

	if err != nil {
		return fmt.Errorf("failed to read messages: %w", err)
	}

	if len(msgs) == 0 {
		cmd.Printf("No messages in %s\n", channel)
		return nil
	}

	cmd.Printf("Messages in %s:\n", channel)
	for _, msg := range msgs {
		ts := msg.Timestamp.Format("2006-01-02 15:04:05")
		cmd.Printf("[%s] <%s> %s\n", ts, msg.From, msg.Content)
	}

	return nil
}

func watchMessages(ctx context.Context, cmd *cobra.Command, channel string) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	store := chatInstance.Store()
	logger := log.FromContext(ctx).WithPrefix("chat-watch")

	// Track displayed message IDs
	displayedIDs := make(map[string]bool)

	// If watching DMs (no channel specified or @user)
	if channel == "" || strings.HasPrefix(channel, "@") {
		return watchDMs(ctx, cmd, chatInstance, user, channel)
	}

	// For channels, load existing messages and mark as displayed
	msgs, err := store.ReadChannelMsgs(channel, types.ReadOptions{
		Limit: 100,
		Types: []string{"msg"},
	})
	if err != nil {
		return fmt.Errorf("failed to read messages: %w", err)
	}

	// Mark existing messages as displayed
	for _, msg := range msgs {
		if msg.ID != "" {
			displayedIDs[msg.ID] = true
		}
	}

	fmt.Printf("Watching %s for new messages (press Ctrl+C to stop)...\n", channel)
	logger.Debug("started watching channel", "channel", channel, "existing_messages", len(msgs))

	// Poll for new messages
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			msgs, err := store.ReadChannelMsgs(channel, types.ReadOptions{
				Limit: 50,
				Types: []string{"msg"},
			})
			if err != nil {
				logger.Debug("error reading messages", "error", err)
				continue
			}

			for _, msg := range msgs {
				if msg.ID != "" && displayedIDs[msg.ID] {
					continue
				}

				if msg.ID != "" {
					displayedIDs[msg.ID] = true
				}

				// Skip own messages
				if msg.From == user.Username() {
					continue
				}

				ts := msg.Timestamp.Format("15:04:05")
				fmt.Printf("[%s] <%s> %s\n", ts, msg.From, msg.Content)
				logger.Debug("displayed message", "id", msg.ID, "from", msg.From)
			}
		}
	}
}

func watchDMs(ctx context.Context, cmd *cobra.Command, chatInstance *chat.Chat, user proto.User, peer string) error {
	store := chatInstance.Store()
	logger := log.FromContext(ctx).WithPrefix("chat-watch")

	// Track displayed message IDs
	displayedIDs := make(map[string]bool)

	// Load existing DMs and mark as displayed
	opts := types.ReadOptions{
		Limit: 100,
		Types: []string{"dm"},
	}
	if peer != "" {
		opts.Peer = peer
	}

	msgs, err := store.ReadUserMsgs(user.Username(), opts)
	if err != nil {
		return fmt.Errorf("failed to read DMs: %w", err)
	}

	// Mark existing messages as displayed
	for _, msg := range msgs {
		if msg.ID != "" {
			displayedIDs[msg.ID] = true
		}
	}

	if peer != "" {
		fmt.Printf("Watching DMs with %s (press Ctrl+C to stop)...\n", peer)
	} else {
		fmt.Println("Watching for new DMs (press Ctrl+C to stop)...")
	}
	logger.Debug("started watching DMs", "peer", peer, "existing_messages", len(msgs))

	// Poll for new messages
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			msgs, err := store.ReadUserMsgs(user.Username(), opts)
			if err != nil {
				logger.Debug("error reading DMs", "error", err)
				continue
			}

			for _, msg := range msgs {
				if msg.ID != "" && displayedIDs[msg.ID] {
					continue
				}

				if msg.ID != "" {
					displayedIDs[msg.ID] = true
				}

				ts := msg.Timestamp.Format("15:04:05")
				fmt.Printf("[%s] <%s> %s\n", ts, msg.Peer, msg.Content)
				logger.Debug("displayed DM", "id", msg.ID, "peer", msg.Peer)
			}
		}
	}
}

func interactiveChat(ctx context.Context, cmd *cobra.Command) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	logger := log.FromContext(ctx).WithPrefix("chat-interactive")
	store := chatInstance.Store()
	username := user.Username()

	// Track displayed message IDs
	displayedIDs := make(map[string]bool)

	// Current channel
	currentChannel := ""

	// Watch mode - output incoming messages
	if chatWatchMode {
		fmt.Println("Entering watch mode. Press Ctrl+C to stop.")

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		reader := bufio.NewReader(os.Stdin)

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				// Check for new messages in current channel
				if currentChannel != "" && strings.HasPrefix(currentChannel, "#") {
					msgs, err := store.ReadChannelMsgs(currentChannel, types.ReadOptions{
						Limit: 50,
						Types: []string{"msg"},
					})
					if err != nil {
						continue
					}

					for _, msg := range msgs {
						if msg.ID != "" && displayedIDs[msg.ID] {
							continue
						}

						if msg.ID != "" {
							displayedIDs[msg.ID] = true
						}

						// Skip own messages
						if msg.From == username {
							continue
						}

						ts := msg.Timestamp.Format("15:04:05")
						fmt.Printf("\n[%s] <%s> %s\n%s> ", ts, msg.From, msg.Content, currentChannel)
					}
				}
			default:
				// Check for input (non-blocking)
				if reader.Buffered() > 0 {
					line, _ := reader.ReadString('\n')
					line = strings.TrimSpace(line)
					if line != "" {
						processChatLine(ctx, line, store, chatInstance, username, &currentChannel, displayedIDs, logger)
					}
				}
			}
		}
	}

	// Non-watch mode - list unread DMs and exit
	return listUnreadDMs(ctx, store, username, displayedIDs)
}

// processInputReader processes commands from an io.Reader
func processInputReader(ctx context.Context, cmd *cobra.Command, r io.Reader, source string) error {
	chatInstance, user, err := getChatAndUser(ctx)
	if err != nil {
		return err
	}

	logger := log.FromContext(ctx).WithPrefix("chat-input")
	store := chatInstance.Store()
	username := user.Username()
	currentChannel := ""
	displayedIDs := make(map[string]bool)

	logger.Debug("processing input from reader", "source", source)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		processChatLine(ctx, line, store, chatInstance, username, &currentChannel, displayedIDs, logger)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	return nil
}

// processInputFile processes commands from a file
func processInputFile(ctx context.Context, cmd *cobra.Command, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	return processInputReader(ctx, cmd, file, filename)
}

func processChatLine(ctx context.Context, line string, store types.ChatStore, chatInstance *chat.Chat, username string, currentChannel *string, displayedIDs map[string]bool, logger *log.Logger) {
	// Commands start with /
	if strings.HasPrefix(line, "/") {
		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])
		args := parts[1:]

		switch cmd {
		case "/join":
			if len(args) > 0 {
				ch := args[0]
				if !strings.HasPrefix(ch, "#") {
					ch = "#" + ch
				}
				*currentChannel = ch
				// Mark existing messages as displayed
				msgs, _ := store.ReadChannelMsgs(ch, types.ReadOptions{Limit: 100, Types: []string{"msg"}})
				for _, msg := range msgs {
					if msg.ID != "" {
						displayedIDs[msg.ID] = true
					}
				}
				fmt.Printf("Joined %s\n", ch)
				logger.Debug("joined channel", "channel", ch)
			}
		case "/leave":
			if len(args) > 0 {
				ch := args[0]
				if !strings.HasPrefix(ch, "#") {
					ch = "#" + ch
				}
				if *currentChannel == ch {
					*currentChannel = ""
				}
				fmt.Printf("Left %s\n", ch)
			}
		case "/select":
			if len(args) > 0 {
				*currentChannel = args[0]
				fmt.Printf("Selected %s\n", *currentChannel)
			}
		case "/channels":
			channels, _ := store.ListChannels()
			fmt.Println("Channels:")
			for _, ch := range channels {
				fmt.Printf("  %s\n", ch)
			}
		case "/help":
			fmt.Println("Commands: /join #channel, /leave #channel, /select #channel, /channels, /help")
			fmt.Println("Messages: Type anything not starting with / to send")
		default:
			fmt.Printf("Unknown command: %s\n", cmd)
		}
		return
	}

	// Regular message - send to current channel
	if *currentChannel == "" {
		fmt.Println("Not in any channel. Use /join #channel first.")
		return
	}

	msgID := chatInstance.GenerateMessageID()
	err := store.AppendChannelMsg(*currentChannel, &types.ChannelMessage{
		ID:        msgID,
		Type:      types.ChannelMsgMessage,
		From:      username,
		Content:   line,
		Timestamp: time.Now(),
	})

	if err != nil {
		fmt.Printf("Failed to send message: %s\n", err)
		return
	}

	displayedIDs[msgID] = true
	ts := time.Now().Format("15:04:05")
	fmt.Printf("[%s] <%s> %s\n", ts, username, line)
	logger.Debug("sent message", "id", msgID, "channel", *currentChannel)
}

func listUnreadDMs(ctx context.Context, store types.ChatStore, username string, displayedIDs map[string]bool) error {
	msgs, err := store.ReadUserMsgs(username, types.ReadOptions{
		Limit: 100,
		Types: []string{"dm"},
	})
	if err != nil {
		return fmt.Errorf("failed to read DMs: %w", err)
	}

	if len(msgs) == 0 {
		fmt.Println("No DMs")
		return nil
	}

	fmt.Println("Recent DMs:")
	for _, msg := range msgs {
		ts := msg.Timestamp.Format("2006-01-02 15:04:05")
		fmt.Printf("[%s] <%s> %s\n", ts, msg.Peer, msg.Content)
	}

	return nil
}

// ChatHandler handles SSH chat sessions with custom protocol
func ChatHandler(s ssh.Session) error {
	ctx := s.Context()
	chatInstance := chat.FromContext(ctx)
	if chatInstance == nil {
		fmt.Fprintln(s, "Chat is not enabled")
		return nil
	}

	user := proto.UserFromContext(ctx)
	if user == nil {
		fmt.Fprintln(s, "Not authenticated")
		return nil
	}

	store := chatInstance.Store()
	username := user.Username()
	logger := log.FromContext(ctx).WithPrefix("chat-handler")

	// Track state
	currentChannel := ""
	displayedIDs := make(map[string]bool)

	// Read commands from session
	reader := bufio.NewReader(s)

	// Send welcome
	fmt.Fprintf(s, "Welcome to chat, %s!\n", username)
	fmt.Fprintln(s, "Commands: /join #channel, /leave, /help, /quit")
	fmt.Fprintln(s, "Use /watch to enter watch mode")

	for {
		prompt := "> "
		if currentChannel != "" {
			prompt = currentChannel + "> "
		}
		fmt.Fprint(s, prompt)

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for quit
		if line == "/quit" || line == "/exit" {
			fmt.Fprintln(s, "Goodbye!")
			return nil
		}

		// Check for watch mode
		if line == "/watch" {
			if currentChannel == "" {
				fmt.Fprintln(s, "Not in any channel. Use /join #channel first.")
				continue
			}
			fmt.Fprintf(s, "Watching %s for new messages (send empty line to stop)...\n", currentChannel)
			logger.Debug("entered watch mode", "channel", currentChannel)

			// Watch loop
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					msgs, err := store.ReadChannelMsgs(currentChannel, types.ReadOptions{
						Limit: 50,
						Types: []string{"msg"},
					})
					if err != nil {
						continue
					}

					for _, msg := range msgs {
						if msg.ID != "" && displayedIDs[msg.ID] {
							continue
						}

						if msg.ID != "" {
							displayedIDs[msg.ID] = true
						}

						if msg.From == username {
							continue
						}

						ts := msg.Timestamp.Format("15:04:05")
						fmt.Fprintf(s, "\n[%s] <%s> %s\n%s> ", ts, msg.From, msg.Content, currentChannel)
					}
				default:
					// Non-blocking read check
					if reader.Buffered() > 0 {
						nextLine, _ := reader.ReadString('\n')
						if strings.TrimSpace(nextLine) == "" {
							fmt.Fprintln(s, "\nStopped watching.")
							goto watchDone
						}
						// Process the line as a message
						processChatLineSSH(s, ctx, nextLine, store, chatInstance, username, &currentChannel, displayedIDs, logger)
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		watchDone:
			continue
		}

		// Process the line
		processChatLineSSH(s, ctx, line, store, chatInstance, username, &currentChannel, displayedIDs, logger)
	}
}

func processChatLineSSH(s ssh.Session, ctx context.Context, line string, store types.ChatStore, chatInstance *chat.Chat, username string, currentChannel *string, displayedIDs map[string]bool, logger *log.Logger) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	if strings.HasPrefix(line, "/") {
		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])
		args := parts[1:]

		switch cmd {
		case "/join":
			if len(args) > 0 {
				ch := args[0]
				if !strings.HasPrefix(ch, "#") {
					ch = "#" + ch
				}
				*currentChannel = ch
				// Mark existing messages as displayed
				msgs, _ := store.ReadChannelMsgs(ch, types.ReadOptions{Limit: 100, Types: []string{"msg"}})
				for _, msg := range msgs {
					if msg.ID != "" {
						displayedIDs[msg.ID] = true
					}
				}
				fmt.Fprintf(s, "Joined %s\n", ch)
				logger.Debug("joined channel via SSH", "channel", ch, "user", username)

				// Show recent messages
				if len(msgs) > 0 {
					fmt.Fprintf(s, "Recent messages in %s:\n", ch)
					for _, msg := range msgs {
						ts := msg.Timestamp.Format("15:04:05")
						fmt.Fprintf(s, "[%s] <%s> %s\n", ts, msg.From, msg.Content)
					}
				}
			} else {
				fmt.Fprintln(s, "Usage: /join #channel")
			}
		case "/leave":
			if *currentChannel != "" {
				fmt.Fprintf(s, "Left %s\n", *currentChannel)
				*currentChannel = ""
			}
		case "/select":
			if len(args) > 0 {
				*currentChannel = args[0]
				fmt.Fprintf(s, "Selected %s\n", *currentChannel)
			}
		case "/channels":
			channels, _ := store.ListChannels()
			fmt.Fprintln(s, "Channels:")
			for _, ch := range channels {
				fmt.Fprintf(s, "  %s\n", ch)
			}
		case "/help":
			fmt.Fprintln(s, "Commands:")
			fmt.Fprintln(s, "  /join #channel - Join a channel")
			fmt.Fprintln(s, "  /leave         - Leave current channel")
			fmt.Fprintln(s, "  /channels      - List all channels")
			fmt.Fprintln(s, "  /watch         - Watch for new messages")
			fmt.Fprintln(s, "  /quit          - Exit chat")
		default:
			fmt.Fprintf(s, "Unknown command: %s\n", cmd)
		}
		return
	}

	// Regular message
	if *currentChannel == "" {
		fmt.Fprintln(s, "Not in any channel. Use /join #channel first.")
		return
	}

	msgID := chatInstance.GenerateMessageID()
	err := store.AppendChannelMsg(*currentChannel, &types.ChannelMessage{
		ID:        msgID,
		Type:      types.ChannelMsgMessage,
		From:      username,
		Content:   line,
		Timestamp: time.Now(),
	})

	if err != nil {
		fmt.Fprintf(s, "Failed to send message: %s\n", err)
		return
	}

	displayedIDs[msgID] = true
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(s, "[%s] <%s> %s\n", ts, username, line)
	logger.Debug("sent message via SSH", "id", msgID, "channel", *currentChannel, "user", username)
}
