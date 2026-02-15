package chat

// Channel types are now in the types package.
// This file is kept for channel-specific logic that doesn't belong in types.

// ChannelSnapshot represents a snapshot of channel state.
type ChannelSnapshot struct {
	Topic   string   `json:"topic"`
	Members []string `json:"members"`
}
