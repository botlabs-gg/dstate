package dstate

import (
	"errors"
	"github.com/jonas747/discordgo"
	"sync"
	"time"
)

var (
	ErrMemberNotFound  = errors.New("Member not found")
	ErrChannelNotFound = errors.New("Channel not found")
)

type GuildState struct {
	sync.RWMutex

	// ID is never mutated, so can be accessed without locking
	ID int64 `json:"id"`

	// The underlying guild, the members and channels fields shouldnt be used
	Guild *discordgo.Guild `json:"guild"`

	Members  map[int64]*MemberState  `json:"members"`
	Channels map[int64]*ChannelState `json:"channels" `

	MaxMessages           int           // Absolute max number of messages cached in a channel
	MaxMessageDuration    time.Duration // Max age of messages, if 0 ignored. (Only checks age whena new message is received on the channel)
	RemoveDeletedMessages bool
	RemoveOfflineMembers  bool
}

// NewGuildstate creates a new guild state, it only uses the passed state to get settings from
// Pass nil to use default settings
func NewGuildState(guild *discordgo.Guild, state *State) *GuildState {
	gCop := new(discordgo.Guild)
	*gCop = *guild

	guildState := &GuildState{
		ID:       guild.ID,
		Guild:    gCop,
		Members:  make(map[int64]*MemberState),
		Channels: make(map[int64]*ChannelState),
	}

	if state != nil {
		guildState.MaxMessages = state.MaxChannelMessages
		guildState.MaxMessageDuration = state.MaxMessageAge
		guildState.RemoveDeletedMessages = state.RemoveDeletedMessages
		guildState.RemoveOfflineMembers = state.RemoveOfflineMembers
	}

	for _, channel := range gCop.Channels {
		guildState.ChannelAddUpdate(false, channel)
	}

	if state.TrackMembers {
		for _, member := range gCop.Members {
			guildState.MemberAddUpdate(false, member)
		}

		for _, presence := range gCop.Presences {
			guildState.PresenceAddUpdate(false, presence)
		}
	}

	gCop.Presences = nil
	gCop.Members = nil

	return guildState
}

// StrID is the same as above but formats it in a string first
func (g *GuildState) StrID() string {
	return discordgo.StrID(g.ID)
}

// GuildUpdate updates the guild with new guild information
func (g *GuildState) GuildUpdate(lock bool, newGuild *discordgo.Guild) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	if newGuild.Roles == nil {
		newGuild.Roles = g.Guild.Roles
	}
	if newGuild.Emojis == nil {
		newGuild.Emojis = g.Guild.Emojis
	}
	if newGuild.VoiceStates == nil {
		newGuild.VoiceStates = g.Guild.VoiceStates
	}
	if newGuild.MemberCount == 0 {
		newGuild.MemberCount = g.Guild.MemberCount
	}

	// Create/update new channels
	*g.Guild = *newGuild
	for _, c := range newGuild.Channels {
		g.ChannelAddUpdate(false, c)
	}

	// Remove removed channels
	if newGuild.Channels != nil {
	OUTER:
		for _, checking := range g.Channels {
			for _, c := range newGuild.Channels {
				if c.ID == checking.ID {
					continue OUTER
				}
			}

			delete(g.Channels, checking.ID)
		}
	}
}

// LightCopy returns a light copy of the inner guild (no slices)
func (g *GuildState) LightCopy(lock bool) *discordgo.Guild {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	gCopy := new(discordgo.Guild)

	*gCopy = *g.Guild
	gCopy.Members = nil
	gCopy.Presences = nil
	gCopy.Channels = nil
	gCopy.VoiceStates = nil
	gCopy.Roles = nil

	return gCopy
}

// Member returns a the member from an id, or nil if not found
func (g *GuildState) Member(lock bool, id int64) *MemberState {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	return g.Members[id]
}

// MemberCopy returns a full copy of a MemberState, this can be used without locking
// Warning: modifying slices in the state (such as roles) causes race conditions, they're only safe to access
func (g *GuildState) MemberCopy(lock bool, id int64) *MemberState {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	m := g.Member(false, id)
	if m == nil {
		return nil
	}

	return m.Copy()
}

// ChannelCopy returns a copy of a channel
// if deep is true, permissionoverwrites will be copied, otherwise nil
func (g *GuildState) ChannelCopy(lock bool, id int64, deep bool) *ChannelState {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	c := g.Channel(false, id)
	if c == nil {
		return nil
	}

	return c.Copy(false, deep)
}

// MemberAddUpdate adds or updates a member
func (g *GuildState) MemberAddUpdate(lock bool, newMember *discordgo.Member) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	existing, ok := g.Members[newMember.User.ID]
	if ok {
		existing.UpdateMember(newMember)
	} else {
		ms := &MemberState{
			Guild: g,
			ID:    newMember.User.ID,
			Bot:   newMember.User.Bot,
		}

		ms.UpdateMember(newMember)
		g.Members[newMember.User.ID] = ms
	}
}

// MemberAdd adds a member to the GuildState
// It differs from addupdate in that it first increases the membercount and then calls MemberAddUpdate
// so it should only be called on the GuildMemberAdd event
func (g *GuildState) MemberAdd(lock bool, newMember *discordgo.Member) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	g.Guild.MemberCount++
	g.MemberAddUpdate(false, newMember)
}

// MemberRemove removes a member from the guildstate
// it also decrements membercount, so only call this on the GuildMemberRemove event
// If you wanna remove a member purely from the state, use StateRemoveMember
func (g *GuildState) MemberRemove(lock bool, id int64) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	g.Guild.MemberCount--
	delete(g.Members, id)
}

// StateRemoveMember removes a guildmember from the state and does NOT decrement member_count
func (g *GuildState) StateRemoveMember(lock bool, id int64) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	delete(g.Members, id)
}

// PresenceAddUpdate adds or updates a presence
func (g *GuildState) PresenceAddUpdate(lock bool, newPresence *discordgo.Presence) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	existing, ok := g.Members[newPresence.User.ID]
	if ok {
		existing.UpdatePresence(newPresence)
	} else {
		if newPresence.Status == discordgo.StatusOffline {
			// Don't bother if this is the case, most likely just removed from the server and the state would be very incomplete
			return
		}

		ms := &MemberState{
			Guild: g,
			ID:    newPresence.User.ID,
			Bot:   newPresence.User.Bot,
		}

		ms.UpdatePresence(newPresence)
		g.Members[newPresence.User.ID] = ms
	}

	if newPresence.Status == discordgo.StatusOffline && g.RemoveOfflineMembers {
		// Remove after a minute incase they just restart the client or something
		time.AfterFunc(time.Minute, func() {
			g.Lock()
			defer g.Unlock()

			member := g.Member(false, newPresence.User.ID)
			if member != nil {
				if !member.PresenceSet || member.PresenceStatus == StatusOffline {
					delete(g.Members, newPresence.User.ID)
				}
			}
		})
	}
}

func copyPresence(in *discordgo.Presence) *discordgo.Presence {
	cop := new(discordgo.Presence)
	*cop = *in

	if in.Game != nil {
		cop.Game = new(discordgo.Game)
		*cop.Game = *in.Game
	}

	cop.User = new(discordgo.User)
	*cop.User = *in.User

	cop.Roles = nil
	if in.Roles != nil {
		cop.Roles = make([]int64, len(in.Roles))
		copy(cop.Roles, in.Roles)
	}

	return cop
}

// Channel retrieves a channelstate by id
func (g *GuildState) Channel(lock bool, id int64) *ChannelState {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	return g.Channels[id]
}

// ChannelAddUpdate adds or updates a channel in the guildstate
func (g *GuildState) ChannelAddUpdate(lock bool, newChannel *discordgo.Channel) *ChannelState {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	existing, ok := g.Channels[newChannel.ID]
	if ok {
		// Patch
		existing.Update(false, newChannel)
		return existing
	}

	state := NewChannelState(g, g, newChannel)
	g.Channels[newChannel.ID] = state

	return state
}

// ChannelRemove removes a channel from the GuildState
func (g *GuildState) ChannelRemove(lock bool, id int64) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}
	delete(g.Channels, id)
}

// Role returns a role by id
func (g *GuildState) Role(lock bool, id int64) *discordgo.Role {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	for _, role := range g.Guild.Roles {
		if role.ID == id {
			return role
		}
	}

	return nil
}

func (g *GuildState) RoleAddUpdate(lock bool, newRole *discordgo.Role) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	existing := g.Role(false, newRole.ID)
	if existing != nil {
		*existing = *newRole
	} else {
		g.Guild.Roles = append(g.Guild.Roles, newRole)
	}
}

func (g *GuildState) RoleRemove(lock bool, id int64) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	for i, v := range g.Guild.Roles {
		if v.ID == id {
			g.Guild.Roles = append(g.Guild.Roles[:i], g.Guild.Roles[i+1:]...)
			return
		}
	}
}

func (g *GuildState) VoiceState(lock bool, userID int64) *discordgo.VoiceState {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	for _, v := range g.Guild.VoiceStates {
		if v.UserID == userID {
			return v
		}
	}

	return nil
}

func (g *GuildState) VoiceStateUpdate(lock bool, update *discordgo.VoiceStateUpdate) {
	if lock {
		g.Lock()
		defer g.Unlock()
	}

	// Handle Leaving Channel
	if update.ChannelID == 0 {
		for i, state := range g.Guild.VoiceStates {
			if state.UserID == update.UserID {
				g.Guild.VoiceStates = append(g.Guild.VoiceStates[:i], g.Guild.VoiceStates[i+1:]...)
				return
			}
		}
	}

	existing := g.VoiceState(false, update.UserID)
	if existing != nil {
		*existing = *update.VoiceState
		return
	}

	vsCopy := new(discordgo.VoiceState)
	*vsCopy = *update.VoiceState

	g.Guild.VoiceStates = append(g.Guild.VoiceStates, vsCopy)

	return
}

// Calculates the permissions for a member.
// https://support.discordapp.com/hc/en-us/articles/206141927-How-is-the-permission-hierarchy-structured-
func (g *GuildState) MemberPermissions(lock bool, channelID int64, memberID int64) (apermissions int, err error) {
	if lock {
		g.RLock()
		defer g.RUnlock()
	}

	if memberID == g.Guild.OwnerID {
		return discordgo.PermissionAll, nil
	}

	mState := g.Member(false, memberID)
	if mState == nil {
		return 0, ErrMemberNotFound
	}

	for _, role := range g.Guild.Roles {
		if role.ID == g.Guild.ID {
			apermissions |= role.Permissions
			break
		}
	}

	for _, role := range g.Guild.Roles {
		for _, roleID := range mState.Roles {
			if role.ID == roleID {
				apermissions |= role.Permissions
				break
			}
		}
	}

	// Administrator bypasses channel overrides
	if apermissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		apermissions |= discordgo.PermissionAll
		return
	}

	cState := g.Channel(false, channelID)
	if cState == nil {
		err = ErrChannelNotFound
		return
	}

	// Apply @everyone overrides from the channel.
	for _, overwrite := range cState.PermissionOverwrites {
		if g.Guild.ID == overwrite.ID {
			apermissions &= ^overwrite.Deny
			apermissions |= overwrite.Allow
			break
		}
	}

	denies := 0
	allows := 0

	// Member overwrites can override role overrides, so do two passes
	for _, overwrite := range cState.PermissionOverwrites {
		for _, roleID := range mState.Roles {
			if overwrite.Type == "role" && roleID == overwrite.ID {
				denies |= overwrite.Deny
				allows |= overwrite.Allow
				break
			}
		}
	}

	apermissions &= ^denies
	apermissions |= allows

	for _, overwrite := range cState.PermissionOverwrites {
		if overwrite.Type == "member" && overwrite.ID == memberID {
			apermissions &= ^overwrite.Deny
			apermissions |= overwrite.Allow
			break
		}
	}

	if apermissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		apermissions |= discordgo.PermissionAllChannel
	}

	return
}
