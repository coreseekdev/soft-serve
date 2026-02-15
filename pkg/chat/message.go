// Package chat provides IRC-like chat functionality for soft-serve.
package chat

import (
	"github.com/charmbracelet/soft-serve/pkg/chat/types"
)

// Re-export types for convenience
type (
	UserMessageType    = types.UserMessageType
	UserMessage        = types.UserMessage
	UserState          = types.UserState
	ChannelMessageType = types.ChannelMessageType
	ChannelMessage     = types.ChannelMessage
	Channel            = types.Channel
	ReadOptions        = types.ReadOptions
	ChatStore          = types.ChatStore
)

// Re-export constants
const (
	UserMsgDM      = types.UserMsgDM
	UserMsgChannel = types.UserMsgChannel
	UserMsgMention = types.UserMsgMention
	UserMsgSub     = types.UserMsgSub
	UserMsgUnsub   = types.UserMsgUnsub
	UserMsgMark    = types.UserMsgMark
	UserMsgAck     = types.UserMsgAck
	UserMsgSnapshot = types.UserMsgSnapshot

	ChannelMsgMessage  = types.ChannelMsgMessage
	ChannelMsgJoin     = types.ChannelMsgJoin
	ChannelMsgLeave    = types.ChannelMsgLeave
	ChannelMsgTopic    = types.ChannelMsgTopic
	ChannelMsgMembers  = types.ChannelMsgMembers
	ChannelMsgSnapshot = types.ChannelMsgSnapshot

	DefaultReadLimit = types.DefaultReadLimit
)

// Re-export variables and functions
var (
	DefaultChannels   = types.DefaultChannels
	NewUserMessage    = types.NewUserMessage
	NewChannelMessage = types.NewChannelMessage
	NewChannel        = types.NewChannel
)
