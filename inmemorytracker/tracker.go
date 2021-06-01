package inmemorytracker

import (
	"container/list"
	"sync"
	"time"

	"github.com/jonas747/discordgo"
	"github.com/jonas747/dstate/v3"
)

type TrackerConfig struct {
	ChannelMessageLen int
	ChannelMessageDur time.Duration

	ChannelMessageLimitsF func(guildID int64) (int, time.Duration)

	// RemoveOfflineMembers      bool
	RemoveOfflineMembersAfter time.Duration
}

type InMemoryTracker struct {
	totalShards int64
	shards      []*ShardTracker
	// conf   TrackerConfig
}

func NewInMemoryTracker(conf TrackerConfig, totalShards int64) *InMemoryTracker {
	shards := make([]*ShardTracker, totalShards)
	for i := range shards {
		shards[i] = newShard(conf, i)
	}

	return &InMemoryTracker{
		shards:      shards,
		totalShards: totalShards,
	}
}

func (t *InMemoryTracker) HandleEvent(s *discordgo.Session, evt interface{}) {
	shard := t.getShard(int64(s.ShardID))
	shard.HandleEvent(s, evt)
}

func (t *InMemoryTracker) RunGCLoop(interval time.Duration) {
	for _, v := range t.shards {
		go v.runGcLoop(interval)
	}
}

// These are updated less frequently and so we remake the indiv lists on update
// this makes us able to just return a straight reference, since the object is effectively immutable
// TODO: reuse the interface version of this?
type SparseGuildState struct {
	Guild       *dstate.GuildState
	Channels    []*dstate.ChannelState
	Roles       []*discordgo.Role
	Emojis      []*discordgo.Emoji
	VoiceStates []*discordgo.VoiceState
}

func SparseGuildStateFromDstate(gs *dstate.GuildSet) *SparseGuildState {
	return &SparseGuildState{
		Guild:       &gs.GuildState,
		Channels:    gs.Channels,
		Roles:       gs.Roles,
		Emojis:      gs.Emojis,
		VoiceStates: gs.VoiceStates,
	}
}

// returns a new copy of SparseGuildState and the inner Guild
func (s *SparseGuildState) copyGuildSet() *SparseGuildState {
	guildSetCopy := *s
	return &guildSetCopy
}

// returns a new copy of SparseGuildState and the inner Guild
func (s *SparseGuildState) copyGuild() *SparseGuildState {
	guildSetCopy := *s
	innerGuild := *s.Guild

	guildSetCopy.Guild = &innerGuild

	return &guildSetCopy
}

// returns a new copy of SparseGuildState and the channels slice
func (s *SparseGuildState) copyChannels() *SparseGuildState {
	guildSetCopy := *s

	guildSetCopy.Channels = make([]*dstate.ChannelState, len(guildSetCopy.Channels))
	copy(guildSetCopy.Channels, s.Channels)

	return &guildSetCopy
}

// returns a new copy of SparseGuildState and the roles slice
func (s *SparseGuildState) copyRoles() *SparseGuildState {
	guildSetCopy := *s

	guildSetCopy.Roles = make([]*discordgo.Role, len(guildSetCopy.Roles))
	copy(guildSetCopy.Roles, s.Roles)

	return &guildSetCopy
}

// returns a new copy of SparseGuildState and the channels slice
func (s *SparseGuildState) copyVoiceStates() *SparseGuildState {
	guildSetCopy := *s

	guildSetCopy.VoiceStates = make([]*discordgo.VoiceState, len(guildSetCopy.VoiceStates))
	copy(guildSetCopy.VoiceStates, s.VoiceStates)

	return &guildSetCopy
}

func (s *SparseGuildState) channel(id int64) *dstate.ChannelState {
	for _, v := range s.Channels {
		if v.ID == id {
			return v
		}
	}

	return nil
}

type WrappedMember struct {
	lastUpdated time.Time
	dstate.MemberState
}

type ShardTracker struct {
	mu sync.RWMutex

	shardID int

	// Key is GuildID
	guilds  map[int64]*SparseGuildState
	members map[int64][]*WrappedMember

	// Key is ChannelID
	messages map[int64]*list.List

	conf TrackerConfig
}

func newShard(conf TrackerConfig, id int) *ShardTracker {
	return &ShardTracker{
		shardID:  id,
		guilds:   make(map[int64]*SparseGuildState),
		members:  make(map[int64][]*WrappedMember),
		messages: make(map[int64]*list.List),
		conf:     conf,
	}
}

func (tracker *ShardTracker) HandleEvent(s *discordgo.Session, i interface{}) {

	switch evt := i.(type) {
	// Guild events
	case *discordgo.GuildCreate:
		tracker.handleGuildCreate(evt)
	case *discordgo.GuildUpdate:
		tracker.handleGuildUpdate(evt)
	case *discordgo.GuildDelete:
		tracker.handleGuildDelete(evt)

	// Member events
	case *discordgo.GuildMemberAdd:
		tracker.handleMemberCreate(evt)
	case *discordgo.GuildMemberUpdate:
		tracker.handleMemberUpdate(evt.Member)
	case *discordgo.GuildMemberRemove:
		tracker.handleMemberDelete(evt)

	// Channel events
	case *discordgo.ChannelCreate:
		tracker.handleChannelCreateUpdate(evt.Channel)
	case *discordgo.ChannelUpdate:
		tracker.handleChannelCreateUpdate(evt.Channel)
	case *discordgo.ChannelDelete:
		tracker.handleChannelDelete(evt)

	// Role events
	case *discordgo.GuildRoleCreate:
		tracker.handleRoleCreateUpdate(evt.GuildID, evt.Role)
	case *discordgo.GuildRoleUpdate:
		tracker.handleRoleCreateUpdate(evt.GuildID, evt.Role)
	case *discordgo.GuildRoleDelete:
		tracker.handleRoleDelete(evt)

	// Message events
	case *discordgo.MessageCreate:
		tracker.handleMessageCreate(evt)
	case *discordgo.MessageUpdate:
		tracker.handleMessageUpdate(evt)
	case *discordgo.MessageDelete:
		tracker.handleMessageDelete(evt)
	case *discordgo.MessageDeleteBulk:
		tracker.handleMessageDeleteBulk(evt)

	// Other
	case *discordgo.PresenceUpdate:
		tracker.handlePresenceUpdate(evt)
	case *discordgo.VoiceStateUpdate:
		tracker.handleVoiceStateUpdate(evt)
	case *discordgo.Ready:
		tracker.handleReady(evt)
	case *discordgo.GuildEmojisUpdate:
		tracker.handleEmojis(evt)
	default:
		return
	}

	// if s.Debug {
	// 	t := reflect.Indirect(reflect.ValueOf(i)).Type()
	// 	log.Printf("Handled event %s; %#v", t.Name(), i)
	// }
}

///////////////////
// Guild events
///////////////////

func (shard *ShardTracker) handleGuildCreate(gc *discordgo.GuildCreate) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	channels := make([]*dstate.ChannelState, 0, len(gc.Channels))
	for _, v := range gc.Channels {
		channels = append(channels, dstate.ChannelStateFromDgo(v))
	}

	guildState := &SparseGuildState{
		Guild:       dstate.GuildStateFromDgo(gc.Guild),
		Channels:    channels,
		Roles:       gc.Roles,
		Emojis:      gc.Emojis,
		VoiceStates: gc.VoiceStates,
	}

	shard.guilds[gc.ID] = guildState

	for _, v := range gc.Members {
		// problem: the presences in guild does not include a full user object
		// solution: only load presences that also have a corresponding member object
		for _, p := range gc.Presences {
			if p.User.ID == v.User.ID {
				pms := dstate.MemberStateFromPresence(&discordgo.PresenceUpdate{
					Presence: *p,
					GuildID:  gc.ID,
				})
				shard.innerHandlePresenceUpdate(pms, true)
				break
			}
		}

		ms := dstate.MemberStateFromMember(v)
		ms.GuildID = gc.ID
		shard.innerHandleMemberUpdate(ms)
	}
}

func (shard *ShardTracker) handleGuildUpdate(gu *discordgo.GuildUpdate) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	newInnerGuild := dstate.GuildStateFromDgo(gu.Guild)

	if existing, ok := shard.guilds[gu.ID]; ok {
		newSparseGuild := existing.copyGuildSet()

		newInnerGuild.MemberCount = existing.Guild.MemberCount

		newSparseGuild.Guild = newInnerGuild
		shard.guilds[gu.ID] = newSparseGuild
	} else {
		shard.guilds[gu.ID] = &SparseGuildState{
			Guild: newInnerGuild,
		}
	}
}

func (shard *ShardTracker) handleGuildDelete(gd *discordgo.GuildDelete) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if gd.Unavailable {
		if existing, ok := shard.guilds[gd.ID]; ok {
			// Note: only allowed to update guild here as that field has been copied
			newSparseGuild := existing.copyGuild()
			newSparseGuild.Guild.Available = false

			shard.guilds[gd.ID] = newSparseGuild
		}
	} else {
		if existing, ok := shard.guilds[gd.ID]; ok {
			for _, v := range existing.Channels {
				delete(shard.messages, v.ID)
			}
		}

		delete(shard.members, gd.ID)
		delete(shard.guilds, gd.ID)
	}
}

///////////////////
// Channel events
///////////////////

func (shard *ShardTracker) handleChannelCreateUpdate(c *discordgo.Channel) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	gs, ok := shard.guilds[c.GuildID]
	if !ok {
		return
	}

	for i, v := range gs.Channels {
		if v.ID == c.ID {
			newSparseGuild := gs.copyChannels()
			newSparseGuild.Channels[i] = dstate.ChannelStateFromDgo(c)
			return
		}
	}

	// channel was not already in state, we need to add it to the channels slice
	newSparseGuild := gs.copyGuildSet()
	newSparseGuild.Channels = append(newSparseGuild.Channels, dstate.ChannelStateFromDgo(c))

	shard.guilds[c.GuildID] = newSparseGuild
}

func (shard *ShardTracker) handleChannelDelete(c *discordgo.ChannelDelete) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	delete(shard.messages, c.ID)

	gs, ok := shard.guilds[c.GuildID]
	if !ok {
		return
	}

	for i, v := range gs.Channels {
		if v.ID == c.ID {
			newSparseGuild := gs.copyChannels()
			newSparseGuild.Channels = append(newSparseGuild.Channels[:i], newSparseGuild.Channels[i+1:]...)
			shard.guilds[c.GuildID] = newSparseGuild
			return
		}
	}
}

///////////////////
// Role events
///////////////////

func (shard *ShardTracker) handleRoleCreateUpdate(guildID int64, r *discordgo.Role) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	gs, ok := shard.guilds[guildID]
	if !ok {
		return
	}

	for i, v := range gs.Roles {
		if v.ID == r.ID {
			newSparseGuild := gs.copyRoles()
			newSparseGuild.Roles[i] = r
			return
		}
	}

	// role was not already in state, we need to add it to the roles slice
	newSparseGuild := gs.copyGuildSet()
	newSparseGuild.Roles = append(newSparseGuild.Roles, r)

	shard.guilds[guildID] = newSparseGuild
}

func (shard *ShardTracker) handleRoleDelete(r *discordgo.GuildRoleDelete) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	gs, ok := shard.guilds[r.GuildID]
	if !ok {
		return
	}

	for i, v := range gs.Roles {
		if v.ID == r.RoleID {
			newSparseGuild := gs.copyRoles()
			newSparseGuild.Roles = append(newSparseGuild.Roles[:i], newSparseGuild.Roles[i+1:]...)
			shard.guilds[r.GuildID] = newSparseGuild
			return
		}
	}
}

///////////////////
// Member events
///////////////////

func (shard *ShardTracker) handleMemberCreate(m *discordgo.GuildMemberAdd) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	gs, ok := shard.guilds[m.GuildID]
	if !ok {
		return
	}

	newSparseGuild := gs.copyGuild()
	newSparseGuild.Guild.MemberCount++
	shard.guilds[m.GuildID] = newSparseGuild

	shard.innerHandleMemberUpdate(dstate.MemberStateFromMember(m.Member))
}

func (shard *ShardTracker) handleMemberUpdate(m *discordgo.Member) {
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.innerHandleMemberUpdate(dstate.MemberStateFromMember(m))
}

// assumes state is locked
func (shard *ShardTracker) innerHandleMemberUpdate(ms *dstate.MemberState) {

	wrapped := &WrappedMember{
		lastUpdated: time.Now(),
		MemberState: *ms,
	}

	members, ok := shard.members[ms.GuildID]
	if !ok {
		// intialize slice
		shard.members[ms.GuildID] = []*WrappedMember{wrapped}
		return
	}

	for i, v := range members {
		if v.User.ID == ms.User.ID {
			// replace in slice
			wrapped.Presence = v.Presence
			members[i] = wrapped
			return
		}
	}

	// member was not already in state, we need to add it to the members slice
	members = append(members, wrapped)
	shard.members[ms.GuildID] = members
}

func (shard *ShardTracker) handleMemberDelete(mr *discordgo.GuildMemberRemove) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Update the memebr count
	gs, ok := shard.guilds[mr.GuildID]
	if !ok {
		return
	}

	newGS := gs.copyGuild()
	newGS.Guild.MemberCount--
	shard.guilds[mr.GuildID] = newGS

	// remove member from state
	if members, ok := shard.members[mr.GuildID]; ok {
		for i, v := range members {
			if v.User.ID == mr.User.ID {
				shard.members[mr.GuildID] = append(members[:i], members[i+1:]...)
				return
			}
		}
	}

}

///////////////////
// Message events
///////////////////

func (shard *ShardTracker) handleMessageCreate(m *discordgo.MessageCreate) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if m.GuildID == 0 {
		return
	}

	if cl, ok := shard.messages[m.ChannelID]; ok {
		cl.PushBack(dstate.MessageStateFromDgo(m.Message))
	} else {
		cl := list.New()
		cl.PushBack(dstate.MessageStateFromDgo(m.Message))
		shard.messages[m.ChannelID] = cl
	}
}

func (shard *ShardTracker) handleMessageUpdate(m *discordgo.MessageUpdate) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if m.GuildID == 0 {
		return
	}

	if cl, ok := shard.messages[m.ChannelID]; ok {
		for e := cl.Back(); e != nil; e = e.Prev() {
			// do something with e.Value
			cast := e.Value.(*dstate.MessageState)
			if cast.ID == m.ID {
				// Update the message
				cop := *cast

				if m.Content != "" {
					cop.Content = m.Content
				}

				if m.Mentions != nil {
					cop.Mentions = make([]discordgo.User, len(m.Mentions))
					for i, v := range m.Mentions {
						cop.Mentions[i] = *v
					}
				}
				if m.Embeds != nil {
					cop.Embeds = make([]discordgo.MessageEmbed, len(m.Embeds))
					for i, v := range m.Embeds {
						cop.Embeds[i] = *v
					}
				}

				if m.Attachments != nil {
					cop.Attachments = make([]discordgo.MessageAttachment, len(m.Attachments))
					for i, v := range m.Attachments {
						cop.Attachments[i] = *v
					}
				}

				if m.Author != nil {
					cop.Author = *m.Author
				}

				if m.MentionRoles != nil {
					cop.MentionRoles = m.MentionRoles
				}

				e.Value = &cop
				return
				// m.parseTimes(msg.Timestamp, msg.EditedTimestamp)
			}
		}
	}
}

func (shard *ShardTracker) handleMessageDelete(m *discordgo.MessageDelete) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if m.GuildID == 0 {
		return
	}

	if cl, ok := shard.messages[m.ChannelID]; ok {
		for e := cl.Back(); e != nil; e = e.Prev() {
			cast := e.Value.(*dstate.MessageState)

			if cast.ID == m.ID {
				cop := *cast
				cop.Deleted = true
				e.Value = &cop
				return
			}
		}
	}
}

func (shard *ShardTracker) handleMessageDeleteBulk(m *discordgo.MessageDeleteBulk) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if m.GuildID == 0 {
		return
	}

	if cl, ok := shard.messages[m.ChannelID]; ok {
		for e := cl.Back(); e != nil; e = e.Prev() {
			cast := e.Value.(*dstate.MessageState)

			for _, delID := range m.Messages {
				if delID == cast.ID {
					cop := *cast
					cop.Deleted = true
					e.Value = &cop
					break
				}
			}
		}
	}
}

///////////////////
// MISC events
///////////////////

func (shard *ShardTracker) handlePresenceUpdate(p *discordgo.PresenceUpdate) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if p.User == nil {
		return
	}

	shard.innerHandlePresenceUpdate(dstate.MemberStateFromPresence(p), false)
}

func (shard *ShardTracker) innerHandlePresenceUpdate(ms *dstate.MemberState, skipFullUserCheck bool) {

	wrapped := &WrappedMember{
		lastUpdated: time.Now(),
		MemberState: *ms,
	}

	members, ok := shard.members[ms.GuildID]
	if !ok {
		// intialize slice
		if skipFullUserCheck || ms.User.Username != "" {
			// only add to state if we have the user object
			shard.members[ms.GuildID] = []*WrappedMember{wrapped}
		}

		return
	}

	for i, v := range members {
		if v.User.ID == ms.User.ID {
			// replace in slice
			wrapped.Member = v.Member
			if ms.User.Username == "" {
				// carry over user object if needed
				wrapped.User = v.User
			}

			members[i] = wrapped
			return
		}
	}

	// member was not already in state, we need to add it to the members slice
	// but only if enough info is available
	if skipFullUserCheck || ms.User.Username != "" {
		members = append(members, wrapped)
		shard.members[ms.GuildID] = members
	}
}

func (shard *ShardTracker) handleVoiceStateUpdate(p *discordgo.VoiceStateUpdate) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	gs, ok := shard.guilds[p.GuildID]
	if !ok {
		return
	}

	newGS := gs.copyVoiceStates()
	for i, v := range newGS.VoiceStates {
		if v.UserID == p.UserID {
			if p.ChannelID == 0 {
				// Left voice chat entirely, remove us
				newGS.VoiceStates = append(newGS.VoiceStates[:i], newGS.VoiceStates[i+1:]...)
			} else {
				// just changed state
				newGS.VoiceStates[i] = p.VoiceState
			}
			return
		}
	}
}

func (shard *ShardTracker) handleReady(p *discordgo.Ready) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	shard.reset()

	for _, v := range p.Guilds {
		shard.guilds[v.ID] = &SparseGuildState{
			Guild: dstate.GuildStateFromDgo(v),
		}
	}
}

func (shard *ShardTracker) handleEmojis(e *discordgo.GuildEmojisUpdate) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	gs, ok := shard.guilds[e.GuildID]
	if !ok {
		return
	}

	newGS := gs.copyGuildSet()
	newGS.Emojis = make([]*discordgo.Emoji, len(e.Emojis))
	copy(newGS.Emojis, e.Emojis)

	shard.guilds[e.GuildID] = newGS
}

// assumes state is locked
func (shard *ShardTracker) reset() {
	shard.guilds = make(map[int64]*SparseGuildState)
	shard.members = make(map[int64][]*WrappedMember)
	shard.messages = make(map[int64]*list.List)
}
