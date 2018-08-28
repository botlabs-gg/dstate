package dstate

import (
	"github.com/jonas747/discordgo"
	"time"
)

// ChannelState represents a channel's state
type ChannelState struct {
	Owner RWLocker    `json:"-" msgpack:"-"`
	Guild *GuildState `json:"-" msgpack:"-"`

	// These fields never change
	ID   int64                 `json:"id"`
	Type discordgo.ChannelType `json:"type"`

	Name                 string                           `json:"name"`
	Topic                string                           `json:"topic"`
	LastMessageID        int64                            `json:"last_message_id"`
	NSFW                 bool                             `json:"nsfw"`
	Position             int                              `json:"position"`
	Bitrate              int                              `json:"bitrate"`
	PermissionOverwrites []*discordgo.PermissionOverwrite `json:"permission_overwrites"`
	ParentID             int64                            `json:"parent_id"`

	// Recicipient used to never be mutated but in the case with group dm's it can
	Recipients []*discordgo.User `json:"recipients"`

	// Accessing the channel without locking the owner yields undefined behaviour
	Messages []*MessageState `json:"messages"`
}

func NewChannelState(guild *GuildState, owner RWLocker, channel *discordgo.Channel) *ChannelState {

	cs := &ChannelState{
		Owner: owner,
		Guild: guild,

		ID:   channel.ID,
		Type: channel.Type,

		Name:                 channel.Name,
		Topic:                channel.Topic,
		LastMessageID:        channel.LastMessageID,
		NSFW:                 channel.NSFW,
		Position:             channel.Position,
		Bitrate:              channel.Bitrate,
		PermissionOverwrites: channel.PermissionOverwrites,
		ParentID:             channel.ParentID,

		Recipients: channel.Recipients,
	}

	return cs
}

func (c *ChannelState) DGoCopy() *discordgo.Channel {
	channel := &discordgo.Channel{

		ID:   c.ID,
		Type: c.Type,

		Name:                 c.Name,
		Topic:                c.Topic,
		LastMessageID:        c.LastMessageID,
		NSFW:                 c.NSFW,
		Position:             c.Position,
		Bitrate:              c.Bitrate,
		PermissionOverwrites: c.PermissionOverwrites,
		ParentID:             c.ParentID,
		Recipients:           c.Recipients,
	}

	if c.Guild != nil {
		channel.GuildID = c.Guild.ID
	}

	return channel
}

// StrID is a conveniece method for retrieving the id in string form
func (cs *ChannelState) StrID() string {
	return discordgo.StrID(cs.ID)
}

// Recipient returns the channels recipient, if you modify this you get undefined behaviour
// This does no locking UNLESS this is a group dm
//
// In case of group dms, this will return the first recipient
func (cs *ChannelState) Recipient() *discordgo.User {
	if cs.Type == discordgo.ChannelTypeGroupDM {
		cs.Owner.RLock()
		defer cs.Owner.RUnlock()
	}
	if len(cs.Recipients) < 1 {
		return nil
	}

	return cs.Recipients[0]
}

// IsPrivate returns true if the channel is private
// This does no locking as Type is immutable
func (cs *ChannelState) IsPrivate() bool {
	return IsPrivate(cs.Type)
}

// Copy returns a copy of the channel
// if deep is true, permissionoverwrites will be copied
func (c *ChannelState) Copy(lock bool, deep bool) *ChannelState {
	if lock {
		c.Owner.RLock()
		defer c.Owner.RUnlock()
	}

	cop := new(ChannelState)
	*cop = *c

	cop.PermissionOverwrites = nil
	cop.Messages = nil

	if deep {
		for _, pow := range c.PermissionOverwrites {
			powCopy := new(discordgo.PermissionOverwrite)
			*powCopy = *pow
			cop.PermissionOverwrites = append(cop.PermissionOverwrites, pow)
		}
	}

	return cop
}

// Update updates a channel
// Undefined behaviour if owner (guild or state) is not locked
func (c *ChannelState) Update(lock bool, newChannel *discordgo.Channel) {
	if lock {
		c.Owner.Lock()
		defer c.Owner.Unlock()
	}

	if newChannel.PermissionOverwrites != nil {
		c.PermissionOverwrites = newChannel.PermissionOverwrites
	}

	if newChannel.Recipients != nil && c.Type == discordgo.ChannelTypeGroupDM {
		c.Recipients = newChannel.Recipients
	}

	c.Name = newChannel.Name
	c.Topic = newChannel.Topic
	c.LastMessageID = newChannel.LastMessageID
	c.NSFW = newChannel.NSFW
	c.Position = newChannel.Position
	c.Bitrate = newChannel.Bitrate
	c.ParentID = newChannel.ParentID
}

// Message returns a message by id or nil if none found
// The only field safe to query on a message without locking the owner (guild or state) is ID
func (c *ChannelState) Message(lock bool, mID int64) *MessageState {
	if lock {
		c.Owner.RLock()
		defer c.Owner.RUnlock()
	}

	for _, m := range c.Messages {
		if m.Message.ID == mID {
			return m
		}
	}

	return nil
}

// MessageAddUpdate adds or updates an existing message
func (c *ChannelState) MessageAddUpdate(lock bool, msg *discordgo.Message, maxMessages int, maxMessageAge time.Duration) {
	if lock {
		c.Owner.Lock()
		defer c.Owner.Unlock()
	}

	defer c.UpdateMessages(false, maxMessages, maxMessageAge)

	existing := c.Message(false, msg.ID)
	if existing != nil {
		existing.Update(msg)
	} else {
		// make a copy
		// No need to copy author aswell as that isnt mutated
		msgCopy := new(discordgo.Message)
		*msgCopy = *msg

		// Add the new one
		ms := &MessageState{
			Message: msgCopy,
		}

		ms.ParseTimes()
		c.Messages = append(c.Messages, ms)
	}
}

// UpdateMessages checks the messages to make sure they fit max message age and max messages
func (c *ChannelState) UpdateMessages(lock bool, maxMsgs int, maxAge time.Duration) {
	if lock {
		c.Owner.Lock()
		defer c.Owner.Unlock()
	}

	if len(c.Messages) > maxMsgs && maxMsgs != -1 {
		c.Messages = c.Messages[len(c.Messages)-maxMsgs:]
	}

	// Check age
	if maxAge == 0 {
		return
	}

	now := time.Now()
	for i := len(c.Messages) - 1; i >= 0; i-- {
		m := c.Messages[i]

		ts := m.ParsedCreated
		if ts.IsZero() {
			continue
		}

		if now.Sub(ts) > maxAge {
			// All messages before this is old aswell
			// TODO: remove by edited timestamp if set
			c.Messages = c.Messages[i:]
			break
		}
	}
}

// MessageRemove removes a message from the channelstate
// If mark is true the the message will just be marked as deleted and not removed from the state
func (c *ChannelState) MessageRemove(lock bool, messageID int64, mark bool) {
	if lock {
		c.Owner.Lock()
		defer c.Owner.Unlock()
	}

	for i, ms := range c.Messages {
		if ms.Message.ID == messageID {
			if !mark {
				c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)
			} else {
				ms.Deleted = true
			}
			return
		}
	}
}

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
