package dstate

import "time"

// TrackingConfig represents the state tracking config
// here you can tell the state tracker what specifically to track
type TrackingConfig struct {
	TrackChannels        bool
	TrackPrivateChannels bool // Dm's, group DM's etc
	TrackMembers         bool
	TrackRoles           bool
	TrackVoice           bool
	TrackPresences       bool
	TrackMessages        bool

	ThrowAwayDMMessages bool // Don't track dm messages if set

	// Absolute max number of messages stored per channel
	MaxChannelMessages int

	// Removes offline members from the state, requires trackpresences
	RemoveOfflineMembers bool

	// Set to keep messages in the state after they're deleted, they will have a deleted flag set on them instead
	KeepDeletedMessages bool

	// Max duration of messages stored, ignored if 0
	// (Messages gets checked when a new message in the channel comes in)
	MaxMessageAge time.Duration

	// Gives you the ability to grant conditional limits
	CustomMessageLimitProvider MessageLimitProvider

	// How long should user caches be alive
	CacheExpirey time.Duration
}

// DefaultTrackingConfig returns A set of common defaults for the tracking config
func DefaultTrackingConfig() *TrackingConfig {
	return &TrackingConfig{
		TrackChannels:        true,
		TrackPrivateChannels: true,
		TrackMembers:         true,
		TrackRoles:           true,
		TrackVoice:           true,
		TrackPresences:       true,
		KeepDeletedMessages:  true,
		ThrowAwayDMMessages:  true,
		TrackMessages:        true,
		CacheExpirey:         time.Minute,
	}
}

// MessageLimitProvider can be implemented to have custom conditional logic for caching messages for longer durations
// for specific guilds or channels
type MessageLimitProvider interface {
	MessageLimits(cs *ChannelState) (maxMessages int, maxMessageAge time.Duration)
}

type RWLocker interface {
	RLock()
	RUnlock()
	Lock()
	Unlock()
}
