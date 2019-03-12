package dstate

import (
	"github.com/jonas747/discordgo"
	"time"
)

// ChannelState represents a channel's state
type ChannelState struct {
	// These fields never change
	ID        int64       `json:"id"`
	Owner     RWLocker    `json:"-" msgpack:"-"`
	Guild     *GuildState `json:"-" msgpack:"-"`
	IsPrivate bool

	// Mutable fields, use Copy() or lock it
	Name                 string                           `json:"name"`
	Type                 discordgo.ChannelType            `json:"type"`
	Topic                string                           `json:"topic"`
	LastMessageID        int64                            `json:"last_message_id"`
	NSFW                 bool                             `json:"nsfw"`
	Position             int                              `json:"position"`
	Bitrate              int                              `json:"bitrate"`
	PermissionOverwrites []*discordgo.PermissionOverwrite `json:"permission_overwrites"`
	ParentID             int64                            `json:"parent_id"`

	// Safe to access in a copy, but not write to in a copy
	Recipients []*discordgo.User `json:"recipients"`

	// Accessing the channel without locking the owner yields undefined behaviour
	Messages []*MessageState `json:"messages"`

	// The last message edit we didn't have the original message tracked for
	// we don't put those in the state because the ordering would be messed up
	// and there could be some unknown messages before and after
	// but in some cases (embed edits for example) the edit can come before the create event
	// for those edge cases we store the last edited unknown message here, then apply it as an update
	LastUnknownMsgEdit *discordgo.Message `json:"last_unknown_msg_edit"`
}

func NewChannelState(guild *GuildState, owner RWLocker, channel *discordgo.Channel) *ChannelState {

	cs := &ChannelState{
		Owner: owner,
		Guild: guild,

		ID: channel.ID,
		// Type chan change, but the channel can never go from a dm type to a guild type, or vice versa
		// since its usefull to access this without locking, store that here
		IsPrivate: IsPrivate(channel.Type),

		Type:                 channel.Type,
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

// DGoCopy returns a discordgo version of the channel representation
// usefull for legacy api's and whatnot
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

// Copy returns a copy of the channel
// permissionoverwrites will be copied
// note: this is not a deep copy, modifying any of the slices is undefined behaviour,
// reading is fine as they're completely replaced when a update occurs
// (messages is another thing and is not available in this copy, manual management of the lock is needed for that)
func (c *ChannelState) Copy(lock bool) *ChannelState {
	if lock {
		c.Owner.RLock()
		defer c.Owner.RUnlock()
	}

	cop := new(ChannelState)
	*cop = *c

	cop.Messages = nil
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

// Message returns a message REFERENCE by id or nil if none found
// The only field safe to query on a message reference without locking the owner (guild or state) is ID
func (c *ChannelState) Message(lock bool, mID int64) *MessageState {
	if lock {
		c.Owner.RLock()
		defer c.Owner.RUnlock()
	}

	index := c.messageIndex(mID)

	if index == -1 {
		return nil
	}

	return c.Messages[index]
}

// MessageCopy returns a copy of the message specified by id, its safe to read all fields, but it's not safe to modify any
func (c *ChannelState) MessageCopy(lock bool, mID int64) *MessageState {
	if lock {
		c.Owner.RLock()
		defer c.Owner.RUnlock()
	}

	index := c.messageIndex(mID)
	if index == -1 {
		return nil
	}

	return c.Messages[index].Copy()
}

func (c *ChannelState) messageIndex(mID int64) int {
	// since this should be ordered by low-high, maybe we should do a binary search?
	for i, v := range c.Messages {
		if v.ID == mID {
			return i
		}
	}

	return -1
}

// MessageAddUpdate adds or updates an existing message
func (c *ChannelState) MessageAddUpdate(lock bool, msg *discordgo.Message, maxMessages int, maxMessageAge time.Duration, edit bool, updateMessages bool) {
	if lock {
		c.Owner.Lock()
		defer c.Owner.Unlock()
	}

	existingIndex := c.messageIndex(msg.ID)

	if existingIndex != -1 {
		c.Messages[existingIndex].Update(msg)
		return
	}

	if edit {
		c.LastUnknownMsgEdit = msg
		return
	}

	ms := MessageStateFromMessage(msg)

	if c.LastUnknownMsgEdit != nil && c.LastUnknownMsgEdit.ID == ms.ID {
		ms.Update(c.LastUnknownMsgEdit)
		c.LastUnknownMsgEdit = nil
	}

	if maxMessageAge > 0 && time.Since(ms.ParsedCreated) > maxMessageAge {
		// Message was old so don't bother with it
		return
	}

	c.Messages = append(c.Messages, ms)

	if updateMessages {
		c.UpdateMessages(false, maxMessages, maxMessageAge)
	}
}

// UpdateMessages checks the messages to make sure they fit max message age and max messages
func (c *ChannelState) UpdateMessages(lock bool, maxMsgs int, maxAge time.Duration) {
	if lock {
		c.Owner.Lock()
		defer c.Owner.Unlock()
	}

	if len(c.Messages) > maxMsgs && maxMsgs != -1 {
		for i := 0; i < len(c.Messages)-maxMsgs; i++ {
			c.Messages[i] = nil
		}
		c.Messages = c.Messages[len(c.Messages)-maxMsgs:]
	}

	// Check age
	if maxAge == 0 {
		return
	}

	now := time.Now()

	// Iterate reverse, new messages are at the end of the slice so iterate until we hit the first old message
	for i := len(c.Messages) - 1; i >= 0; i-- {
		m := c.Messages[i]

		ts := m.ParsedCreated
		if ts.IsZero() {
			continue
		}

		if now.Sub(ts) > maxAge {
			// All messages before this is old aswell

			// if we don't do this the messages wont be collected for garbage collection since they're still referenced by the underlying array
			for j := 0; j <= i; j++ {
				c.Messages[i] = nil
			}

			c.Messages = c.Messages[i+1:]
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
		if ms.ID == messageID {
			if !mark {
				c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)
			} else {
				ms.Deleted = true
			}
			return
		}
	}
}
