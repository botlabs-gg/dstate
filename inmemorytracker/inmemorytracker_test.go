package inmemorytracker

import (
	"strconv"
	"testing"

	"github.com/jonas747/discordgo"
)

var testSession = &discordgo.Session{ShardID: 0, ShardCount: 1}

const initialTestGuildID = 1
const initialTestChannelID = 10
const initialTestRoleID = 100
const initialTestMemberID = 1000

func createTestChannel(guildID int64, channelID int64, permissionsOverwrites []*discordgo.PermissionOverwrite) *discordgo.Channel {
	return &discordgo.Channel{
		ID:                   channelID,
		GuildID:              guildID,
		Name:                 "test channel-" + strconv.FormatInt(channelID, 10),
		Type:                 discordgo.ChannelTypeGuildText,
		PermissionOverwrites: permissionsOverwrites,
	}
}

func createTestUser(id int64) *discordgo.User {
	return &discordgo.User{
		ID:            id,
		Username:      "test member-" + strconv.FormatInt(id, 10),
		Discriminator: "0000",
	}
}

func createTestMember(guildID int64, id int64, roles []int64) *discordgo.Member {
	return &discordgo.Member{
		GuildID: guildID,
		Roles:   roles,
		User: &discordgo.User{
			ID:            id,
			Username:      "test member-" + strconv.FormatInt(id, 10),
			Discriminator: "0000",
		},
	}
}

func createTestState() *InMemoryTracker {
	state := NewInMemoryTracker(TrackerConfig{}, 1)
	state.HandleEvent(testSession, &discordgo.GuildCreate{
		Guild: &discordgo.Guild{
			ID:          initialTestGuildID,
			Name:        "test guild",
			OwnerID:     initialTestMemberID,
			MemberCount: 1,
			Members: []*discordgo.Member{
				createTestMember(0, initialTestMemberID, []int64{initialTestRoleID}),
			},
			Presences: []*discordgo.Presence{
				{User: createTestUser(initialTestMemberID)},
			},
			Channels: []*discordgo.Channel{
				createTestChannel(0, initialTestChannelID, nil),
			},
			Roles: []*discordgo.Role{
				{ID: initialTestRoleID},
			},
		},
	})

	return state
}

func assertMemberExists(t *testing.T, tracker *InMemoryTracker, guildID int64, memberID int64, checkMember, checkPresence bool) {
	ms := tracker.GetMember(guildID, memberID)
	if ms == nil {
		t.Fatal("ms is nil")
	}

	if checkMember && ms.Member == nil {
		t.Fatal("ms.Member is nil")
	}

	if checkPresence && ms.Presence == nil {
		t.Fatal("ms.presence is nil")
	}
}

func TestGuildCreate(t *testing.T) {
	tracker := createTestState()
	assertMemberExists(t, tracker, 1, initialTestMemberID, true, true)

	gs := tracker.GetGuild(initialTestGuildID)
	if gs == nil {
		t.Fatal("gs is nil")
	}

	if gs.GetRole(initialTestRoleID) == nil {
		t.Fatal("gc role is nil")
	}

	if gs.GetChannel(initialTestChannelID) == nil {
		t.Fatal("gc channel is nil")
	}
}

func TestNoneExistantMember(t *testing.T) {
	tracker := createTestState()
	ms := tracker.GetMember(1, 10001)
	if ms != nil {
		t.Fatal("ms is not nul, should be nil")
	}
}

func TestMemberAdd(t *testing.T) {
	tracker := createTestState()

	tracker.HandleEvent(testSession, &discordgo.GuildMemberAdd{
		Member: createTestMember(1, 1001, nil),
	})

	assertMemberExists(t, tracker, 1, 1001, true, false)

	gs := tracker.GetGuild(1)
	if gs.MemberCount != 2 {
		t.Fatal("Member count not increased:", gs.MemberCount)
	}
}
