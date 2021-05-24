package dstate

import (
	"github.com/jonas747/discordgo"
)

// The state system for yags
// You are safe to read everything returned
// You are NOT safe to modify anything returned, as that can cause race conditions
type StateTracker interface {
	GetGuild(guildID int64) *GuildSet
	GetShardGuilds(shardID int64) []*GuildSet

	GetMember(guildID int64, memberID int64) *MemberState

	// channelID is optional and may be 0 to just return guild permissions
	// Returns false if guild, channel or member was not found
	GetMemberPermissions(guildID int64, channelID int64, memberID int64) (perms int64, ok bool)
	// Returns false if guild or channel was not found
	GetRolePermisisons(guildID int64, channelID int64, memberID int64, roles []int64) (perms int64, ok bool)

	GetMessages(guildID int64, channelID int64) []*MessageState

	// // TODO: Are these needed? should we just use GetGuildSet?
	// // if were using a remote tracker we may just end up caching stuff like this anyways so it should be a cheap operation
	// GetGuild(guildID int64) *GuildState
	// GetChannel(guildID int64, channelID int64) *ChannelState
	// GetRole(guildID int64, roleID int64) *discordgo.Role
	// GetEmoji(guildID int64, emojiID int64) *discordgo.Emoji

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

	// // Calls f on all messages, return true to continue or false to stop
	// //
	// // This is a blocking, non-concurrent operation that returns when f has either returned false or f has been called on all members
	// // it should be safe to modify local caller variables within f without needing any syncronization on the caller side
	// // as syncronization is done by the implmentation to ensure f is never called concurrently
	// //
	// // It's up to the implementation to decide how to chunk the results, it may even just be 1 chunk
	// // The reason it can be chunked is in the cases where state is remote
	// //
	// // Note that f may not be called if there were no results
	// IterateMessages(guildID int64, channelID int64, f func(chunk []*MessageState) bool)

	// // Calls f on all guilds, return true to continue or false to stop
	// //
	// // This is a blocking, non-concurrent operation that returns when f has either returned false or f has been called on all members
	// // it should be safe to modify local caller variables within f without needing any syncronization on the caller side
	// // as syncronization is done by the implmentation to ensure f is never called concurrently
	// //
	// // It's up to the implementation to decide how to chunk the results, it may even just be 1 chunk
	// // The reason it can be chunked is in the cases where state is remote
	// //
	// // Note that f may not be called if there were no results
	// IterateGuilds(guildID int64, f func(chunk []*GuildSet) bool)
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

func (gs *GuildSet) GetMemberPermissions(channelID int64, memberID int64, roles []int64) (perms int64, ok bool) {
	ok = true

	var overwrites []discordgo.PermissionOverwrite

	if channel := gs.GetChannel(channelID); channel != nil {
		overwrites = channel.PermissionOverwrites
	} else if channelID != 0 {
		// we still continue as far as we can with the calculations even though we can't apply channel permissions
		ok = false
	}

	perms = CalculatePermissions(&gs.GuildState, gs.Roles, overwrites, memberID, roles)
	return perms, ok
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
}

func GuildStateFromDgo(guild *discordgo.Guild) *GuildState {
	if guild.Unavailable {
		return &GuildState{
			ID:        guild.ID,
			Available: false,
		}
	}

	return &GuildState{
		ID:          guild.ID,
		Available:   true,
		Region:      guild.Region,
		MemberCount: int64(guild.MemberCount),
		OwnerID:     guild.OwnerID,
	}
}

type ChannelState struct {
	ID       int64
	GuildID  int64
	ParentID int64

	PermissionOverwrites []discordgo.PermissionOverwrite
}

func ChannelStateFromDgo(c *discordgo.Channel) *ChannelState {
	pos := make([]discordgo.PermissionOverwrite, len(c.PermissionOverwrites))
	for i, v := range c.PermissionOverwrites {
		pos[i] = *v
	}

	return &ChannelState{
		ID:                   c.ID,
		GuildID:              c.GuildID,
		PermissionOverwrites: pos,
		ParentID:             c.ParentID,
	}
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

type PresenceFields struct {
	// Acitvity here
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

func MemberStateFromPresence(p *discordgo.PresenceUpdate) *MemberState {
	var user discordgo.User
	if p.User != nil {
		user = *p.User
	}

	return &MemberState{
		User:    user,
		GuildID: p.GuildID,
		Roles:   p.Roles,
		Nick:    p.Nick,

		Member:   nil,
		Presence: &PresenceFields{},
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
}

func MessageStateFromDgo(m *discordgo.Message) *MessageState {
	var embeds []discordgo.MessageEmbed
	if len(m.Embeds) > 0 {
		embeds = make([]discordgo.MessageEmbed, len(m.Embeds))
		for i, v := range m.Embeds {
			embeds[i] = *v
		}
	}

	var mentions []discordgo.User
	if len(m.Mentions) > 0 {
		mentions = make([]discordgo.User, len(m.Mentions))
		for i, v := range m.Mentions {
			mentions[i] = *v
		}
	}

	var attachments []discordgo.MessageAttachment
	if len(m.Attachments) > 0 {
		attachments = make([]discordgo.MessageAttachment, len(m.Attachments))
		for i, v := range m.Attachments {
			attachments[i] = *v
		}
	}

	var author discordgo.User
	if m.Author != nil {
		author = *m.Author
	}

	return &MessageState{
		ID:        m.ID,
		GuildID:   m.GuildID,
		ChannelID: m.ChannelID,
		Author:    author,
		Member:    m.Member,

		Embeds:       embeds,
		Mentions:     mentions,
		Attachments:  attachments,
		MentionRoles: m.MentionRoles,
	}
}
