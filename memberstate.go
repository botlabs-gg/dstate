package dstate

import (
	"encoding/hex"
	"github.com/jonas747/discordgo"
	"strconv"
	"strings"
	"time"
)

type PresenceStatus int32

const (
	StatusNotSet       PresenceStatus = 0
	StatusOnline       PresenceStatus = 1
	StatusIdle         PresenceStatus = 2
	StatusDoNotDisturb PresenceStatus = 3
	StatusInvisible    PresenceStatus = 4
	StatusOffline      PresenceStatus = 5
)

// MemberState represents the state of a member
type MemberState struct {
	Guild *GuildState `json:"-" msgpack:"-"`

	// The ID of the member, safe to access without locking
	ID int64 `json:"id"`

	// The time at which the member joined the guild, in ISO8601.
	// This may be zero if the member hasnt been updated
	JoinedAt time.Time `json:"joined_at"`

	// The nickname of the member, if they have one.
	Nick string `json:"nick"`

	// A list of IDs of the roles which are possessed by the member.
	Roles []int64 `json:"roles"`

	PresenceStatus PresenceStatus  `json:"presence_status"`
	PresenceGame   *discordgo.Game `json:"presence_game"`

	// The users username.
	Username string `json:"username"`

	// The hash of the user's avatar. Use Session.UserAvatar
	// to retrieve the avatar itself.
	Avatar [16]byte `json:"avatar"`
	// The discriminator of the user (4 numbers after name).
	Discriminator int32 `json:"discriminator"`

	AnimatedAvatar bool `json:"animated_avatar"`

	// Whether the user is a bot, safe to access without locking
	Bot       bool `json:"bot"`
	MemberSet bool `json:"member_set"`
	// Wether the presence Information was set
	PresenceSet bool `json:"presence_set"`
}

func MSFromDGoMember(gs *GuildState, member *discordgo.Member) *MemberState {
	ms := &MemberState{
		Guild:     gs,
		ID:        member.User.ID,
		Roles:     member.Roles,
		Username:  member.User.Username,
		Nick:      member.Nick,
		Bot:       member.User.Bot,
		MemberSet: true,
	}

	ms.ParseAvatar(member.User.Avatar)

	discrim, _ := strconv.ParseInt(member.User.Discriminator, 10, 32)
	ms.Discriminator = int32(discrim)

	ms.JoinedAt, _ = time.Parse(time.RFC3339, member.JoinedAt)

	return ms
}

// StrID is the same as above, formatted as a string
func (m *MemberState) StrID() string {
	return discordgo.StrID(m.ID)
}

func (m *MemberState) UpdateMember(member *discordgo.Member) {
	// Patch
	if member.JoinedAt != "" {
		parsed, _ := time.Parse(time.RFC3339, member.JoinedAt)
		m.JoinedAt = parsed
	}

	m.Roles = member.Roles

	// Seems to always be provided
	m.Nick = member.Nick

	m.Username = member.User.Username
	m.ParseAvatar(member.User.Avatar)

	discrim, _ := strconv.ParseInt(member.User.Discriminator, 10, 32)
	m.Discriminator = int32(discrim)

	m.MemberSet = true
}

func (m *MemberState) UpdatePresence(presence *discordgo.Presence) {
	m.PresenceSet = true
	m.PresenceGame = presence.Game

	if !m.MemberSet {
		m.Nick = presence.Nick
	}

	if presence.User.Username != "" {
		m.Username = presence.User.Username
	}

	if presence.User.Discriminator != "" {
		discrim, _ := strconv.ParseInt(presence.User.Discriminator, 10, 32)
		m.Discriminator = int32(discrim)
	}

	if presence.User.Avatar != "" {
		m.ParseAvatar(presence.User.Avatar)
	}

	if presence.Status != "" {

		switch presence.Status {
		case discordgo.StatusOnline:
			m.PresenceStatus = StatusOnline
		case discordgo.StatusIdle:
			m.PresenceStatus = StatusIdle
		case discordgo.StatusDoNotDisturb:
			m.PresenceStatus = StatusDoNotDisturb
		case discordgo.StatusInvisible:
			m.PresenceStatus = StatusInvisible
		case discordgo.StatusOffline:
			m.PresenceStatus = StatusOffline
		}
	}
}

func (m *MemberState) ParseAvatar(str string) {
	if strings.HasPrefix(str, "a_") {
		str = str[2:]
		m.AnimatedAvatar = true
	} else {
		m.AnimatedAvatar = false
	}

	hex.Decode(m.Avatar[:], []byte(str))
}

// Copy returns a copy of the state, this is not a deep copy so the slices will point to the same arrays, so they're only read safe, not write safe
func (m *MemberState) Copy() *MemberState {
	cop := new(MemberState)
	*cop = *m
	return cop
}

func (m *MemberState) StrAvatar() string {
	dst := make([]byte, 32)

	hex.Encode(dst, m.Avatar[:])

	str := string(dst)
	if m.AnimatedAvatar {
		str = "a_" + str
	}

	return str
}

func (m *MemberState) DGoCopy() *discordgo.Member {
	return &discordgo.Member{
		User:     m.DGoUser(),
		Nick:     m.Nick,
		Roles:    m.Roles,
		JoinedAt: m.JoinedAt.Format(time.RFC3339),
	}
}

func (m *MemberState) DGoUser() *discordgo.User {
	user := &discordgo.User{
		ID:            m.ID,
		Username:      m.Username,
		Bot:           m.Bot,
		Avatar:        m.StrAvatar(),
		Discriminator: strconv.FormatInt(int64(m.Discriminator), 10),
	}

	// Pad the discrim
	if m.Discriminator < 10 {
		user.Discriminator = "000" + user.Discriminator
	} else if m.Discriminator < 100 {
		user.Discriminator = "00" + user.Discriminator
	} else if m.Discriminator < 1000 {
		user.Discriminator = "0" + user.Discriminator
	}

	return user
}
