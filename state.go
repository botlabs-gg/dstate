package dstate

import (
	"log"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonas747/discordgo"
)

type State struct {
	sync.RWMutex

	r *discordgo.Ready

	// All connected guilds
	Guilds map[int64]*GuildState

	// Global channel mapping for convenience
	Channels        map[int64]*ChannelState
	PrivateChannels map[int64]*ChannelState

	// Absolute max number of messages stored per channel
	MaxChannelMessages int

	// Max duration of messages stored, ignored if 0
	// (Messages gets checked when a new message in the channel comes in)
	MaxMessageAge time.Duration

	// Gives you the ability to grant conditional limits
	CustomLimitProvider LimitProvider

	TrackChannels        bool
	TrackPrivateChannels bool // Dm's, group DM's etc
	TrackMembers         bool
	TrackRoles           bool
	TrackVoice           bool
	TrackPresences       bool
	TrackMessages        bool
	ThrowAwayDMMessages  bool // Don't track dm messages if set

	// Removes offline members from the state, requires trackpresences
	RemoveOfflineMembers bool

	// Set to remove deleted messages from state
	KeepDeletedMessages bool

	// Enabled debug logging
	Debug bool

	// How long guild user caches should be active
	CacheExpiry time.Duration

	// Cache statistics
	cacheMiss *int64
	cacheHits *int64

	cacheEvictedTotal    int64
	membersEvictedTotal  int64
	messagesRemovedTotal int64
}

func NewState() *State {
	return &State{
		Guilds:          make(map[int64]*GuildState),
		Channels:        make(map[int64]*ChannelState),
		PrivateChannels: make(map[int64]*ChannelState),

		TrackChannels:        true,
		TrackPrivateChannels: true,
		TrackMembers:         true,
		TrackRoles:           true,
		TrackVoice:           true,
		TrackPresences:       true,
		KeepDeletedMessages:  true,
		ThrowAwayDMMessages:  true,
		TrackMessages:        true,

		cacheMiss: new(int64),
		cacheHits: new(int64),

		CacheExpiry: time.Minute,
	}
}

// Guild returns a given guilds GuildState
func (s *State) Guild(lock bool, id int64) *GuildState {
	if lock {
		s.RLock()
		defer s.RUnlock()
	}

	return s.Guilds[id]
}

// LightGuildcopy returns a light copy of a guild (without any slices included)
func (s *State) LightGuildCopy(lock bool, id int64) *discordgo.Guild {
	if lock {
		s.RLock()
	}

	guild := s.Guild(false, id)
	if guild == nil {
		if lock {
			s.RUnlock()
		}
		return nil
	}

	if lock {
		s.RUnlock()
	}

	return guild.LightCopy(true)
}

// Channel returns a channelstate from id
func (s *State) Channel(lock bool, id int64) *ChannelState {
	if lock {
		s.RLock()
		defer s.RUnlock()
	}

	return s.Channels[id]
}

// ChannelCopy returns a copy of a channel,
// lock dictates wether state should be RLocked or not, channel will be locked regardless
// All slices on the copy are safe to read, but not write to
func (s *State) ChannelCopy(lock bool, id int64) *ChannelState {

	cState := s.Channel(lock, id)
	if cState == nil {
		return nil
	}

	return cState.Copy(true)
}

// Differantiate between create and update
func (s *State) GuildCreate(lock bool, g *discordgo.Guild) {
	if lock {
		s.Lock()
		defer s.Unlock()
	}

	// Preserve messages in the state and
	// purge existing global channel maps if this guy was already in the state
	preservedMessages := make(map[int64][]*MessageState)

	existing := s.Guild(false, g.ID)
	if existing != nil {
		// Synchronization is hard
		toRemove := make([]int64, 0)
		s.Unlock()
		existing.RLock()
		for _, channel := range existing.Channels {
			preservedMessages[channel.ID] = channel.Messages
			toRemove = append(toRemove, channel.ID)
		}
		existing.RUnlock()
		s.Lock()

		for _, cID := range toRemove {
			delete(s.Channels, cID)
		}
	}

	// No need to lock it since we just created it and theres no chance of anyone else accessing it
	guildState := NewGuildState(g, s)
	if existing != nil && existing.userCache != nil {
		guildState.userCache = existing.userCache
	}

	for _, channel := range guildState.Channels {
		if preserved, ok := preservedMessages[channel.ID]; ok {
			channel.Messages = preserved
		}

		s.Channels[channel.ID] = channel
	}

	s.Guilds[g.ID] = guildState
}

func (s *State) GuildUpdate(lockMain bool, g *discordgo.Guild) {
	guildState := s.Guild(lockMain, g.ID)
	if guildState == nil {
		s.GuildCreate(true, g)
		return
	}

	guildState.GuildUpdate(true, g)
}

func (s *State) GuildRemove(id int64) {
	s.Lock()
	defer s.Unlock()

	g := s.Guild(false, id)
	if g == nil {
		return
	}
	// Remove all references
	for c, cs := range s.Channels {
		if cs.Guild == g {
			delete(s.Channels, c)
		}
	}
	delete(s.Guilds, id)
}

func (s *State) HandleReady(r *discordgo.Ready) {
	s.Lock()
	defer s.Unlock()

	s.r = r

	if s.TrackPrivateChannels {
		for _, channel := range r.PrivateChannels {
			cs := NewChannelState(nil, &sync.RWMutex{}, channel)
			s.Channels[channel.ID] = cs
			s.PrivateChannels[channel.ID] = cs
		}
	}

	for _, v := range r.Guilds {
		// Can't update the guild here if it exists already because out own guild is all zeroed out in the ready
		// event for bot account.
		if s.Guild(false, v.ID) == nil {
			s.GuildCreate(false, v)
		}
	}
}

// User Returns a copy of the user from the ready event
func (s *State) User(lock bool) *discordgo.SelfUser {
	if lock {
		s.RLock()
		defer s.RUnlock()
	}

	if s.r == nil || s.r.User == nil {
		return nil
	}

	uCopy := new(discordgo.SelfUser)
	*uCopy = *s.r.User

	return uCopy
}

func (s *State) ChannelAddUpdate(newChannel *discordgo.Channel) {
	if !s.TrackChannels {
		return
	}

	c := s.Channel(true, newChannel.ID)
	if c != nil {
		c.Update(true, newChannel)
		return
	}

	if !IsPrivate(newChannel.Type) {
		g := s.Guild(true, newChannel.GuildID)
		if g != nil {
			c = g.ChannelAddUpdate(true, newChannel)
		} else {
			// Happens occasionally when leaving guilds
			return
		}
	} else {
		if !s.TrackPrivateChannels {
			return
		}
		// Belongs to no guild, so we can create a new rwmutex
		c = NewChannelState(nil, &sync.RWMutex{}, newChannel)
	}

	s.Lock()
	s.Channels[newChannel.ID] = c
	if IsPrivate(newChannel.Type) {
		s.PrivateChannels[newChannel.ID] = c
	}
	s.Unlock()
}

func (s *State) ChannelRemove(evt *discordgo.Channel) {
	if !s.TrackChannels {
		return
	}

	if IsPrivate(evt.Type) {
		s.Lock()
		defer s.Unlock()

		delete(s.Channels, evt.ID)
		delete(s.PrivateChannels, evt.ID)
		return
	}

	g := s.Guild(true, evt.GuildID)
	if g != nil {
		g.ChannelRemove(true, evt.ID)

		s.Lock()
		delete(s.Channels, evt.ID)
		s.Unlock()
	}
}

func (s *State) HandleEvent(session *discordgo.Session, i interface{}) {

	handled := false
	if s.Debug {
		t := reflect.Indirect(reflect.ValueOf(i)).Type()
		defer func() {
			if !handled {
				log.Printf("Did not handle, or had issues with %s; %#v", t.Name(), i)
			}
		}()
		log.Println("Inc event ", t.Name())
	}

	switch evt := i.(type) {

	// Guild events
	case *discordgo.GuildCreate:
		s.GuildCreate(true, evt.Guild)
	case *discordgo.GuildUpdate:
		s.GuildUpdate(true, evt.Guild)
	case *discordgo.GuildDelete:
		if !evt.Unavailable {
			s.GuildRemove(evt.ID)
		}

	// Member events
	case *discordgo.GuildMemberAdd:
		if !s.TrackMembers {
			return
		}

		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.MemberAdd(true, evt.Member)
		}
	case *discordgo.GuildMemberUpdate:
		if !s.TrackMembers {
			return
		}

		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.MemberAddUpdate(true, evt.Member)
		}
	case *discordgo.GuildMemberRemove:
		if !s.TrackMembers {
			return
		}

		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.MemberRemove(true, evt.User.ID)
		}

	// Channel events
	case *discordgo.ChannelCreate:
		s.ChannelAddUpdate(evt.Channel)
	case *discordgo.ChannelUpdate:
		s.ChannelAddUpdate(evt.Channel)
	case *discordgo.ChannelDelete:
		s.ChannelRemove(evt.Channel)

	// Role events
	case *discordgo.GuildRoleCreate:
		if !s.TrackRoles {
			return
		}

		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.RoleAddUpdate(true, evt.Role)
		}
	case *discordgo.GuildRoleUpdate:
		if !s.TrackRoles {
			return
		}

		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.RoleAddUpdate(true, evt.Role)
		}
	case *discordgo.GuildRoleDelete:
		if !s.TrackRoles {
			return
		}

		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.RoleRemove(true, evt.RoleID)
		}

	// Message events
	case *discordgo.MessageCreate:
		if !s.TrackMessages {
			return
		}

		channel := s.Channel(true, evt.ChannelID)
		if channel == nil {
			return
		}
		if channel.IsPrivate && s.ThrowAwayDMMessages {
			return
		}

		channel.MessageAddUpdate(true, evt.Message, false)
	case *discordgo.MessageUpdate:
		if !s.TrackMessages {
			return
		}

		channel := s.Channel(true, evt.ChannelID)
		if channel == nil {
			return
		}
		if channel.IsPrivate && s.ThrowAwayDMMessages {
			return
		}

		channel.MessageAddUpdate(true, evt.Message, true)
	case *discordgo.MessageDelete:
		if !s.TrackMessages {
			return
		}

		channel := s.Channel(true, evt.ChannelID)
		if channel == nil {
			return
		}
		if channel.IsPrivate && s.ThrowAwayDMMessages {
			return
		}
		channel.MessageRemove(true, evt.Message.ID, s.KeepDeletedMessages)
	case *discordgo.MessageDeleteBulk:
		if !s.TrackMessages {
			return
		}

		channel := s.Channel(true, evt.ChannelID)
		if channel == nil {
			return
		}
		if channel.IsPrivate && s.ThrowAwayDMMessages {
			return
		}
		channel.Owner.Lock()
		defer channel.Owner.Unlock()

		for _, v := range evt.Messages {
			channel.MessageRemove(false, v, s.KeepDeletedMessages)
		}

	// Other
	case *discordgo.PresenceUpdate:
		if !s.TrackPresences {
			return
		}

		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.PresenceAddUpdate(true, &evt.Presence)
		}
	case *discordgo.VoiceStateUpdate:
		if !s.TrackVoice {
			return
		}
		g := s.Guild(true, evt.GuildID)
		if g != nil {
			g.VoiceStateUpdate(true, evt.VoiceState)
		}
	case *discordgo.Ready:
		s.HandleReady(evt)
	default:
		handled = true
		return
	}

	handled = true

	if s.Debug {
		t := reflect.Indirect(reflect.ValueOf(i)).Type()
		log.Printf("Handled event %s; %#v", t.Name(), i)
	}
}

func (s *State) RunGCWorker() {
	for {
		s.runGC()
	}
}

// GuildIterationTime is how many seconds GC should *roughly* take to go gc all guilds
var GuildIterationTime = 60 * 30

func (s *State) runGC() {
	// just for safety
	time.Sleep(time.Millisecond * 10)

	// Get a copy of all the guild states, that way we dont need to keep the main guild store locked
	guilds := make([]*GuildState, 0, 1000)
	s.RLock()
	for _, v := range s.Guilds {
		guilds = append(guilds, v)
	}
	s.RUnlock()

	// Start chewing through em
	// we sleep 100ms between each process, and make sure we've gotten through each guild in GuildIterationTime seconds
	processPerInterval := len(guilds) / (GuildIterationTime * 10)
	if processPerInterval < 1 {
		processPerInterval = 1
	}

	processedNow := 0
	evicted := 0
	membersRemoved := 0
	messagesRemoved := 0
	for _, g := range guilds {
		processedNow++

		if processedNow >= processPerInterval {
			time.Sleep(time.Millisecond * 100)
			processedNow = 0
		}

		maxMessages := s.MaxChannelMessages
		maxMessageAge := s.MaxMessageAge
		if s.CustomLimitProvider != nil {
			maxMessages, maxMessageAge = s.CustomLimitProvider.MessageLimits(g)
		}

		mr, ev, msgR := g.runGC(s.CacheExpiry, s.RemoveOfflineMembers, maxMessages, maxMessageAge)
		evicted += ev
		membersRemoved += mr
		messagesRemoved += msgR
	}

	s.Lock()
	s.cacheEvictedTotal += int64(evicted)
	s.membersEvictedTotal += int64(membersRemoved)
	s.messagesRemovedTotal += int64(messagesRemoved)
	s.Unlock()
}

func (s *State) GuildsSlice(lock bool) []*GuildState {
	if lock {
		s.RLock()
		defer s.RUnlock()
	}

	dst := make([]*GuildState, 0, len(s.Guilds))
	for _, g := range s.Guilds {
		dst = append(dst, g)
	}

	return dst
}

type StateStats struct {
	CacheHits, CacheMisses int64
	UserCachceEvictedTotal int64
	MembersRemovedTotal    int64
	MessagesRemovedTotal   int64
}

func (s *State) CacheStats() (hit, miss int64) {
	hit = atomic.LoadInt64(s.cacheHits)
	miss = atomic.LoadInt64(s.cacheMiss)
	return
}

func (s *State) StateStats() *StateStats {
	hits, misses := s.CacheStats()
	s.RLock()
	defer s.RUnlock()

	return &StateStats{
		CacheHits:              hits,
		CacheMisses:            misses,
		UserCachceEvictedTotal: s.cacheEvictedTotal,
		MembersRemovedTotal:    s.membersEvictedTotal,
		MessagesRemovedTotal:   s.messagesRemovedTotal,
	}
}

type RWLocker interface {
	RLock()
	RUnlock()
	Lock()
	Unlock()
}

type LimitProvider interface {
	MessageLimits(gs *GuildState) (maxMessages int, maxMessageAge time.Duration)
}
