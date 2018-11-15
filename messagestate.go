package dstate

import (
	"github.com/jonas747/discordgo"
	"time"
)

// MessageState represents the state of a message
type MessageState struct {
	Message *discordgo.Message

	// Set it the message was deleted
	Deleted bool

	// The parsed times below are cached because parsing all messages
	// timestamps in state everytime a new one came in would be stupid
	ParsedCreated time.Time
	ParsedEdited  time.Time
}

// ParseTimes parses the created and edited timestamps
func (m *MessageState) ParseTimes() {
	// The discord api is stopid, and edits can come before creates
	// Can also be handled before even if received in order cause of goroutines and scheduling
	if m.Message.Timestamp != "" {
		parsedC, _ := m.Message.Timestamp.Parse()
		m.ParsedCreated = parsedC
	}

	if m.Message.EditedTimestamp != "" {
		parsedE, _ := m.Message.EditedTimestamp.Parse()
		m.ParsedEdited = parsedE
	}
}

// Copy returns a copy of the message, that can be used without further locking the owner
func (m *MessageState) Copy(deep bool) *discordgo.Message {
	mCopy := new(discordgo.Message)
	*mCopy = *m.Message

	mCopy.Author = nil
	mCopy.Attachments = nil
	mCopy.Embeds = nil
	mCopy.MentionRoles = nil
	mCopy.Mentions = nil
	mCopy.Reactions = nil

	if !deep {
		return mCopy
	}

	if m.Message.Author != nil {
		mCopy.Author = new(discordgo.User)
		*mCopy.Author = *m.Message.Author
	}

	mCopy.Attachments = append(mCopy.Attachments, m.Message.Attachments...)
	mCopy.Embeds = append(mCopy.Embeds, m.Message.Embeds...)
	mCopy.Reactions = append(mCopy.Reactions, m.Message.Reactions...)

	mCopy.MentionRoles = append(mCopy.MentionRoles, m.Message.MentionRoles...)
	mCopy.Mentions = append(mCopy.Mentions, m.Message.Mentions...)

	return mCopy
}

func (m *MessageState) Update(msg *discordgo.Message) {
	// Patch the m message
	if msg.Content != "" {
		m.Message.Content = msg.Content
	}
	if msg.EditedTimestamp != "" {
		m.Message.EditedTimestamp = msg.EditedTimestamp
	}
	if msg.Mentions != nil {
		m.Message.Mentions = msg.Mentions
	}
	if msg.Embeds != nil {
		m.Message.Embeds = msg.Embeds
	}
	if msg.Attachments != nil {
		m.Message.Attachments = msg.Attachments
	}
	if msg.Timestamp != "" {
		m.Message.Timestamp = msg.Timestamp
	}
	if msg.Author != nil {
		m.Message.Author = msg.Author
	}
	m.ParseTimes()
}

func IsPrivate(t discordgo.ChannelType) bool {
	return t == discordgo.ChannelTypeGroupDM || t == discordgo.ChannelTypeDM
}

func deepCopyMessage(in *discordgo.Message) *discordgo.Message {
	baseCopy := *in

	if in.Author != nil {
		authorCop := *in.Author
		baseCopy.Author = &authorCop
	}

	// deep copy mentioned roles
	baseCopy.MentionRoles = make([]int64, len(in.MentionRoles))
	copy(baseCopy.MentionRoles, in.MentionRoles)

	// deep copy attachments
	baseCopy.Attachments = make([]*discordgo.MessageAttachment, len(in.Attachments))
	for i, v := range in.Attachments {
		cop := *v
		baseCopy.Attachments[i] = &cop
	}

	// deep copy mentioned users
	baseCopy.Mentions = make([]*discordgo.User, len(in.Mentions))
	for i, v := range in.Mentions {
		cop := *v
		baseCopy.Mentions[i] = &cop
	}

	// reaction  tracking not implemented
	baseCopy.Reactions = nil

	// deep copy embeds
	baseCopy.Embeds = make([]*discordgo.MessageEmbed, len(in.Embeds))
	for i, v := range in.Embeds {
		cop := *v
		cop.Fields = make([]*discordgo.MessageEmbedField, len(v.Fields))
		for k, f := range v.Fields {
			fc := *f
			cop.Fields[k] = &fc
		}

		if v.Footer != nil {
			cc := *v.Footer
			cop.Footer = &cc
		}
		if v.Image != nil {
			cc := *v.Image
			cop.Image = &cc
		}
		if v.Thumbnail != nil {
			cc := *v.Thumbnail
			cop.Thumbnail = &cc
		}
		if v.Video != nil {
			cc := *v.Video
			cop.Video = &cc
		}
		if v.Provider != nil {
			cc := *v.Provider
			cop.Provider = &cc
		}
		if v.Author != nil {
			cc := *v.Author
			cop.Author = &cc
		}

		baseCopy.Embeds[i] = &cop
	}

	return &baseCopy
}
