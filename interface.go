package dstate

import (
	"strconv"
	"strings"
	"time"

	"github.com/jonas747/discordgo"
)

// The state system for yags
// You are safe to read everything returned
// You are NOT safe to modify anything returned, as that can cause race conditions
type StateTracker interface {
	// GetGuild returns a guild set for the provided guildID, or nil if not found
	GetGuild(guildID int64) *GuildSet

	// GetShardGuilds returns all the guild sets on the shard
	// this will panic if shardID is below 0 or >= total shards
	GetShardGuilds(shardID int64) []*GuildSet

	// GetMember returns a member from state
	// Note that MemberState.Member is nil if only presence data is present, and likewise for MemberState.Presence
	//
	// returns nil if member is not found in the guild's state
	// which does not mean they're not a member, simply that they're not cached
	GetMember(guildID int64, memberID int64) *MemberState

	// GetMessages returns the messages of the channel, up to limit, you may pass in a pre-allocated buffer to save allocations.
	// If cap(buf) is less than the needed then a new one will be created and returned
	// if len(buf) is greater than needed, it will be sliced to the proper length
	GetMessages(guildID int64, channelID int64, query *MessagesQuery) []*MessageState

	// Calls f on all members, return true to continue or false to stop
	//
	// This is a blocking, non-concurrent operation that returns when f has either returned false or f has been called on all members
	// it should be safe to modify local caller variables within f without needing any syncronization on the caller side
	// as syncronization is done by the implmentation to ensure f is never called concurrently
	//
	// It's up to the implementation to decide how to chunk the results, it may even just be 1 chunk
	// The reason it can be chunked is in the cases where state is remote
	//
	// Note that f may not be called if there were no results
	IterateMembers(guildID int64, f func(chunk []*MemberState) bool)
}

// Relatively cheap, less frequently updated things
// thinking: should we keep voice states in here? those are more frequently updated but ehhh should we?
type GuildSet struct {
	GuildState

	Channels    []*ChannelState
	Roles       []*discordgo.Role
	Emojis      []*discordgo.Emoji
	VoiceStates []*discordgo.VoiceState
}

func (gs *GuildSet) GetMemberPermissions(channelID int64, memberID int64, roles []int64) (perms int64, err error) {

	var overwrites []discordgo.PermissionOverwrite

	if channel := gs.GetChannel(channelID); channel != nil {
		overwrites = channel.PermissionOverwrites
	} else if channelID != 0 {
		// we still continue as far as we can with the calculations even though we can't apply channel permissions
		err = &ErrChannelNotFound{
			ChannelID: channelID,
		}
	}

	perms = CalculatePermissions(&gs.GuildState, gs.Roles, overwrites, memberID, roles)
	return perms, err
}

func (gs *GuildSet) GetChannel(id int64) *ChannelState {
	for _, v := range gs.Channels {
		if v.ID == id {
			return v
		}
	}

	return nil
}

func (gs *GuildSet) GetRole(id int64) *discordgo.Role {
	for _, v := range gs.Roles {
		if v.ID == id {
			return v
		}
	}

	return nil
}

func (gs *GuildSet) GetVoiceState(userID int64) *discordgo.VoiceState {
	for _, v := range gs.VoiceStates {
		if v.UserID == userID {
			return v
		}
	}

	return nil
}

func (gs *GuildSet) GetEmoji(id int64) *discordgo.Emoji {
	for _, v := range gs.Emojis {
		if v.ID == id {
			return v
		}
	}

	return nil
}

type GuildState struct {
	ID          int64
	Available   bool
	MemberCount int64
	OwnerID     int64
	Region      string
	Name        string
	Icon        string
}

type ChannelState struct {
	ID       int64
	GuildID  int64
	ParentID int64
	Name     string
	Topic    string
	Type     discordgo.ChannelType
	NSFW     bool
	Position int
	Bitrate  int

	PermissionOverwrites []discordgo.PermissionOverwrite
}

func (c *ChannelState) IsPrivate() bool {
	if c.Type == discordgo.ChannelTypeDM || c.Type == discordgo.ChannelTypeGroupDM {
		return true
	}

	return false
}

// A fully cached member
type MemberState struct {
	// All the sparse fields are always available
	User    discordgo.User
	GuildID int64
	Roles   []int64
	Nick    string

	// These are not always available and all usages should be checked
	Member   *MemberFields
	Presence *PresenceFields
}

type MemberFields struct {
	JoinedAt discordgo.Timestamp
}

type PresenceStatus int32

const (
	StatusNotSet       PresenceStatus = 0
	StatusOnline       PresenceStatus = 1
	StatusIdle         PresenceStatus = 2
	StatusDoNotDisturb PresenceStatus = 3
	StatusInvisible    PresenceStatus = 4
	StatusOffline      PresenceStatus = 5
)

type PresenceFields struct {
	// Acitvity here
	Game   *LightGame
	Status PresenceStatus
}

type LightGame struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Details string `json:"details,omitempty"`
	State   string `json:"state,omitempty"`

	Type discordgo.GameType `json:"type"`
}

func MemberStateFromMember(member *discordgo.Member) *MemberState {
	var user discordgo.User
	if member.User != nil {
		user = *member.User
	}

	return &MemberState{
		User:    user,
		GuildID: member.GuildID,
		Roles:   member.Roles,
		Nick:    member.Nick,

		Member: &MemberFields{
			JoinedAt: member.JoinedAt,
		},
		Presence: nil,
	}
}

func (ms *MemberState) DgoMember() *discordgo.Member {
	m := &discordgo.Member{
		GuildID:  ms.GuildID,
		JoinedAt: ms.Member.JoinedAt,
		Nick:     ms.Nick,
		Roles:    ms.Roles,
		User:     &ms.User,
	}

	if ms.Member != nil {
		m.JoinedAt = ms.Member.JoinedAt
	}

	return m
}

type MessageState struct {
	ID        int64
	GuildID   int64
	ChannelID int64

	Author  discordgo.User
	Member  *discordgo.Member
	Content string

	Embeds       []discordgo.MessageEmbed
	Mentions     []discordgo.User
	MentionRoles []int64
	Attachments  []discordgo.MessageAttachment

	ParsedCreatedAt time.Time
	ParsedEditedAt  time.Time

	Deleted bool
}

func (m *MessageState) ContentWithMentionsReplaced() string {
	content := m.Content

	for _, user := range m.Mentions {
		content = strings.NewReplacer(
			"<@"+strconv.FormatInt(user.ID, 10)+">", "@"+user.Username,
			"<@!"+strconv.FormatInt(user.ID, 10)+">", "@"+user.Username,
		).Replace(content)
	}

	return content
}

var _ error = (*ErrGuildNotFound)(nil)

type ErrGuildNotFound struct {
	GuildID int64
}

func (e *ErrGuildNotFound) Error() string {
	return "Guild not found: " + strconv.FormatInt(e.GuildID, 10)
}

var _ error = (*ErrChannelNotFound)(nil)

type ErrChannelNotFound struct {
	ChannelID int64
}

func (e *ErrChannelNotFound) Error() string {
	return "Channel not found: " + strconv.FormatInt(e.ChannelID, 10)
}

// IsGuildNotFound returns true if a ErrGuildNotFound, and also the GuildID if it was
func IsGuildNotFound(e error) (bool, int64) {
	if gn, ok := e.(*ErrGuildNotFound); ok {
		return true, gn.GuildID
	}

	return false, 0
}

// IsChannelNotFound returns true if a ErrChannelNotFound, and also the ChannelID if it was
func IsChannelNotFound(e error) (bool, int64) {
	if cn, ok := e.(*ErrChannelNotFound); ok {
		return true, cn.ChannelID
	}

	return false, 0
}

type MessagesQuery struct {
	Buf []*MessageState

	// Get messages made before this ID (message_id <  before)
	Before int64

	// Get messages made after this ID (message_id > after)
	After int64

	Limit          int
	IncludeDeleted bool
}
