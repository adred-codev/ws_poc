package main

import (
	"fmt"
	"regexp"
	"strings"
)

// =============================================================================
// CHANNEL MAPPING (NATS ↔ WebSocket)
// =============================================================================
// Maps between NATS subjects and WebSocket channels with validation.
//
// Architecture:
// Publisher (API) → NATS subjects → WebSocket Server → WebSocket channels → Clients
//
// NATS Subject Structure (what publisher publishes to):
//   odin.token.{tokenId}    - All token events (trade, liquidity, metadata, comments)
//   odin.user.{userId}      - All user events (favorites, balances)
//   odin.global             - System-wide events (new listings, featured)
//
// WebSocket Channel Structure (what clients subscribe to):
//   token.{tokenId}         - Simplified (drops "odin." prefix)
//   user.{userId}           - User-specific private data
//   global                  - Global broadcasts
//
// Examples:
//   NATS: odin.token.BTC123  → WebSocket: token.BTC123
//   NATS: odin.user.alice    → WebSocket: user.alice
//   NATS: odin.global        → WebSocket: global
//
// See: /Volumes/Dev/Codev/Toniq/ws_poc/docs/events/TOKEN_UPDATE_EVENTS.md

// Channel types
const (
	ChannelTypeToken  = "token"
	ChannelTypeUser   = "user"
	ChannelTypeGlobal = "global"
)

// Channel validation patterns
var (
	// Token channel: token.{tokenId}
	// tokenId can be alphanumeric with underscores/hyphens
	tokenChannelPattern = regexp.MustCompile(`^token\.([a-zA-Z0-9_-]+)$`)

	// User channel: user.{userId}
	// userId can be alphanumeric with underscores/hyphens
	userChannelPattern = regexp.MustCompile(`^user\.([a-zA-Z0-9_-]+)$`)

	// Global channel: exactly "global"
	globalChannelPattern = regexp.MustCompile(`^global$`)
)

// NATSSubjectToChannel converts a NATS subject to a WebSocket channel
//
// Mappings:
//   odin.token.{tokenId} → token.{tokenId}
//   odin.user.{userId}   → user.{userId}
//   odin.global          → global
//
// Returns empty string if subject doesn't match expected format.
func NATSSubjectToChannel(subject string) string {
	// Remove "odin." prefix
	if !strings.HasPrefix(subject, "odin.") {
		return "" // Invalid: must start with "odin."
	}

	channel := strings.TrimPrefix(subject, "odin.")

	// Validate the resulting channel format
	if IsValidChannel(channel) {
		return channel
	}

	return "" // Invalid format
}

// ChannelToNATSSubject converts a WebSocket channel to a NATS subject
//
// Mappings:
//   token.{tokenId} → odin.token.{tokenId}
//   user.{userId}   → odin.user.{userId}
//   global          → odin.global
//
// Returns empty string if channel is invalid.
func ChannelToNATSSubject(channel string) string {
	if !IsValidChannel(channel) {
		return ""
	}

	return "odin." + channel
}

// IsValidChannel checks if a channel name follows the naming convention
//
// Valid formats:
//   token.{tokenId}  - Token updates (e.g., "token.BTC123")
//   user.{userId}    - User-specific data (e.g., "user.alice")
//   global           - System-wide events
func IsValidChannel(channel string) bool {
	return tokenChannelPattern.MatchString(channel) ||
		userChannelPattern.MatchString(channel) ||
		globalChannelPattern.MatchString(channel)
}

// ParseChannel extracts the channel type and identifier from a channel string
//
// Examples:
//   "token.BTC123" → ("token", "BTC123")
//   "user.alice"   → ("user", "alice")
//   "global"       → ("global", "")
//
// Returns empty strings if channel is invalid.
func ParseChannel(channel string) (channelType string, identifier string) {
	if matches := tokenChannelPattern.FindStringSubmatch(channel); matches != nil {
		return ChannelTypeToken, matches[1]
	}

	if matches := userChannelPattern.FindStringSubmatch(channel); matches != nil {
		return ChannelTypeUser, matches[1]
	}

	if globalChannelPattern.MatchString(channel) {
		return ChannelTypeGlobal, ""
	}

	return "", ""
}

// BuildTokenChannel constructs a token channel name
//
// Example: BuildTokenChannel("BTC123") → "token.BTC123"
func BuildTokenChannel(tokenId string) string {
	return fmt.Sprintf("token.%s", tokenId)
}

// BuildUserChannel constructs a user channel name
//
// Example: BuildUserChannel("alice") → "user.alice"
func BuildUserChannel(userId string) string {
	return fmt.Sprintf("user.%s", userId)
}

// BuildGlobalChannel returns the global channel name
//
// Example: BuildGlobalChannel() → "global"
func BuildGlobalChannel() string {
	return "global"
}

// GetNATSSubscriptionPatterns returns all NATS subject patterns to subscribe to
//
// Returns wildcards for dynamic matching:
//   odin.token.*  - All token events
//   odin.user.*   - All user events
//   odin.global   - Global events
func GetNATSSubscriptionPatterns() []string {
	return []string{
		"odin.token.*", // Matches odin.token.BTC123, odin.token.ETH456, etc.
		"odin.user.*",  // Matches odin.user.alice, odin.user.bob, etc.
		"odin.global",  // Exact match for global events
	}
}

// ValidateChannels validates a list of channels and returns only valid ones
//
// Returns:
//   - valid: slice of valid channels
//   - invalid: slice of invalid channel names
//
// Use this when handling client subscribe requests to filter out malformed channels.
func ValidateChannels(channels []string) (valid []string, invalid []string) {
	valid = make([]string, 0, len(channels))
	invalid = make([]string, 0)

	for _, channel := range channels {
		if IsValidChannel(channel) {
			valid = append(valid, channel)
		} else {
			invalid = append(invalid, channel)
		}
	}

	return valid, invalid
}
