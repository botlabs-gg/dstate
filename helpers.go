package dstate

import (
	"time"

	"github.com/jonas747/discordgo"
)

func GuildSetFromGuild(guild *discordgo.Guild) *GuildSet {

	channels := make([]*ChannelState, 0, len(guild.Channels))
	for _, v := range guild.Channels {
		channels = append(channels, ChannelStateFromDgo(v))
	}

	return &GuildSet{
		GuildState:  *GuildStateFromDgo(guild),
		Channels:    channels,
		Roles:       guild.Roles,
		Emojis:      guild.Emojis,
		VoiceStates: guild.VoiceStates,
	}
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

	parsedC, _ := m.Timestamp.Parse()
	var parsedE time.Time
	if m.EditedTimestamp != "" {
		parsedE, _ = m.EditedTimestamp.Parse()
	}

	return &MessageState{
		ID:        m.ID,
		GuildID:   m.GuildID,
		ChannelID: m.ChannelID,
		Author:    author,
		Member:    m.Member,

		Embeds:          embeds,
		Mentions:        mentions,
		Attachments:     attachments,
		MentionRoles:    m.MentionRoles,
		ParsedCreatedAt: parsedC,
		ParsedEditedAt:  parsedE,
	}
}

func MemberStateFromPresence(p *discordgo.PresenceUpdate) *MemberState {
	var user discordgo.User
	if p.User != nil {
		user = *p.User
	}

	// get the main activity
	// it either gets the first one, or the one with typ 1 (streaming)
	var mainActivity *discordgo.Game
	for i, v := range p.Activities {
		if i == 0 || v.Type == 1 {
			mainActivity = v
		}
	}

	var lg *LightGame
	if mainActivity != nil {
		lg = &LightGame{
			Name:    mainActivity.Name,
			Details: mainActivity.Details,
			URL:     mainActivity.URL,
			State:   mainActivity.State,
			Type:    mainActivity.Type,
		}
	}

	// update the rest
	var status PresenceStatus

	switch p.Status {
	case discordgo.StatusOnline:
		status = StatusOnline
	case discordgo.StatusIdle:
		status = StatusIdle
	case discordgo.StatusDoNotDisturb:
		status = StatusDoNotDisturb
	case discordgo.StatusInvisible:
		status = StatusInvisible
	case discordgo.StatusOffline:
		status = StatusOffline
	}

	return &MemberState{
		User:    user,
		GuildID: p.GuildID,

		Member: nil,
		Presence: &PresenceFields{
			Game:   lg,
			Status: status,
		},
	}
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
		Name:                 c.Name,
		Topic:                c.Topic,
		Type:                 c.Type,
		NSFW:                 c.NSFW,
		Position:             c.Position,
		Bitrate:              c.Bitrate,
	}
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
		Name:        guild.Name,
		Icon:        guild.Icon,
	}
}
