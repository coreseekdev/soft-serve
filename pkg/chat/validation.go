package chat

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	// MaxMessageSize is the maximum allowed message size (32KB)
	MaxMessageSize = 32768
	// MaxChannelNameLength is the maximum channel name length
	MaxChannelNameLength = 64
	// MaxUsernameLength is the maximum username length
	MaxUsernameLength = 32
)

var (
	// validChannelName matches valid channel names (# followed by alphanumeric, hyphens, underscores)
	validChannelName = regexp.MustCompile(`^#[a-zA-Z0-9_-]{1,63}$`)
	// validUsername matches valid usernames (alphanumeric, hyphens, underscores)
	validUsername = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,31}$`)
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s: %s", e.Field, e.Message)
}

// ValidateMessage validates a message content
func ValidateMessage(content string) error {
	// Check message size
	if len(content) > MaxMessageSize {
		return &ValidationError{
			Field:   "content",
			Message: fmt.Sprintf("message too large (max %d bytes)", MaxMessageSize),
		}
	}

	// Check for empty message
	if strings.TrimSpace(content) == "" {
		return &ValidationError{
			Field:   "content",
			Message: "message cannot be empty",
		}
	}

	// Check for excessive control characters
	ctrlCount := 0
	for _, r := range content {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			ctrlCount++
		}
	}
	if ctrlCount > 10 {
		return &ValidationError{
			Field:   "content",
			Message: "message contains too many control characters",
		}
	}

	return nil
}

// ValidateChannelName validates a channel name
func ValidateChannelName(channel string) error {
	if !strings.HasPrefix(channel, "#") {
		return &ValidationError{
			Field:   "channel",
			Message: "channel name must start with #",
		}
	}

	if len(channel) > MaxChannelNameLength {
		return &ValidationError{
			Field:   "channel",
			Message: fmt.Sprintf("channel name too long (max %d chars)", MaxChannelNameLength),
		}
	}

	if !validChannelName.MatchString(channel) {
		return &ValidationError{
			Field:   "channel",
			Message: "channel name can only contain letters, numbers, hyphens, and underscores",
		}
	}

	return nil
}

// ValidateUsername validates a username
func ValidateUsername(username string) error {
	if len(username) > MaxUsernameLength {
		return &ValidationError{
			Field:   "username",
			Message: fmt.Sprintf("username too long (max %d chars)", MaxUsernameLength),
		}
	}

	if !validUsername.MatchString(username) {
		return &ValidationError{
			Field:   "username",
			Message: "username can only contain letters, numbers, hyphens, and underscores",
		}
	}

	return nil
}

// SanitizeMessage removes or replaces dangerous characters
func SanitizeMessage(content string) string {
	// Remove null bytes and other problematic control characters
	result := strings.Builder{}
	for _, r := range content {
		// Keep printable characters and common whitespace
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			result.WriteRune(r)
		}
		// Replace other control characters with space
		else if unicode.IsControl(r) {
			result.WriteRune(' ')
		}
	}
	return result.String()
}
