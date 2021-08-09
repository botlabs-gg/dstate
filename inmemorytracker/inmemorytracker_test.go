package inmemorytracker

import (
	"strconv"
	"testing"
	"time"

	"github.com/jonas747/discordgo"
)

var testSession = &discordgo.Session{ShardID: 0, ShardCount: 1}

const initialTestGuildID = 1
const initialTestChannelID = 10
const initialTestRoleID = 100
const initialTestMemberID = 1000
const initialTestThreadID = 10000
const secondTesThreadID = 50
const initialtestBotID = 100000

func createTestChannel(guildID int64, channelID int64, permissionsOverwrites []*discordgo.PermissionOverwrite) *discordgo.Channel {
	return &discordgo.Channel{
		ID:                   channelID,
		GuildID:              guildID,
		Name:                 "test channel-" + strconv.FormatInt(channelID, 10),
		Type:                 discordgo.ChannelTypeGuildText,
		PermissionOverwrites: permissionsOverwrites,
	}
}

func createTestThread(guildID int64, channelID int64, permissionsOverwrites []*discordgo.PermissionOverwrite) *discordgo.Channel {
	return &discordgo.Channel{
		ID:                   channelID,
		GuildID:              guildID,
		ParentID:             initialTestChannelID,
		ThreadMetadata:       &discordgo.ThreadMetadata{},
		Name:                 "test thread-" + strconv.FormatInt(channelID, 10),
		Type:                 discordgo.ChannelTypeGuildPublicThread,
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

func createTestState(conf TrackerConfig) *InMemoryTracker {
	state := NewInMemoryTracker(conf, 1)
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
			Threads: []*discordgo.Channel{
				createTestThread(0, initialTestThreadID, nil),
			},
			Roles: []*discordgo.Role{
				{ID: initialTestRoleID},
			},
		},
	})

	return state
}

func createTestStateGuildDelete(conf TrackerConfig) *InMemoryTracker {
	state := createTestState(conf)
	state.HandleEvent(testSession, &discordgo.GuildDelete{
		Guild: &discordgo.Guild{
			ID:          initialTestGuildID,
			Name:        "test guild",
			OwnerID:     initialTestMemberID,
			MemberCount: 1,
			Threads: []*discordgo.Channel{
				createTestThread(0, initialTestThreadID, nil),
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
	tracker := createTestState(TrackerConfig{
		BotMemberID: initialtestBotID,
	})
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

	if gs.GetChannel(initialTestThreadID) == nil {
		t.Fatal("gc thread is nil")
	}

	shard := tracker.getGuildShard(gs.ID)
	if shard == nil {
		t.Fatal("shard is nil")
	}

	guildID, ok := shard.threadsGuildID[initialTestThreadID]
	if !ok {
		t.Fatal("shard.threadsGuildID not set on handleGuildCreate")
	}

	if guildID != gs.ID {
		t.Fatal("shard.threadsGuildID was not properly set")
	}
}

func TestGuildDelete(t *testing.T) {
	tracker := createTestStateGuildDelete(TrackerConfig{
		BotMemberID: initialtestBotID,
	})

	gs := tracker.GetGuild(initialTestGuildID)
	if gs != nil {
		t.Fatal("guild still available")
	}

	shard := tracker.getGuildShard(initialTestGuildID)
	if shard == nil {
		t.Fatal("shard is nil")
	}

	_, ok := shard.threadsGuildID[initialTestThreadID]
	if ok {
		t.Fatal("shard.threadsGuildID not deleted on handleGuildDelete")
	}
}

func TestNoneExistantMember(t *testing.T) {
	tracker := createTestState(TrackerConfig{
		BotMemberID: initialtestBotID,
	})
	ms := tracker.GetMember(1, 10001)
	if ms != nil {
		t.Fatal("ms is not nul, should be nil")
	}
}

func TestMemberAdd(t *testing.T) {
	tracker := createTestState(TrackerConfig{
		BotMemberID: initialtestBotID,
	})

	tracker.HandleEvent(testSession, &discordgo.GuildMemberAdd{
		Member: createTestMember(1, 1001, nil),
	})

	assertMemberExists(t, tracker, 1, 1001, true, false)

	gs := tracker.GetGuild(1)
	if gs.MemberCount != 2 {
		t.Fatal("Member count not increased:", gs.MemberCount)
	}
}

func TestChannelUpdate(t *testing.T) {
	tracker := createTestState(TrackerConfig{
		BotMemberID: initialtestBotID,
	})
	channel := tracker.GetGuild(initialTestGuildID).GetChannel(initialTestChannelID)
	if channel == nil {
		t.Fatal("channel not found")
	}

	updt := createTestChannel(1, initialTestChannelID, nil)
	updt.Name = "this is a new name!"

	tracker.HandleEvent(testSession, &discordgo.ChannelUpdate{
		Channel: updt,
	})

	channel = tracker.GetGuild(initialTestGuildID).GetChannel(initialTestChannelID)
	if channel == nil {
		t.Fatal("channel not found")
	}

	if channel.Name != updt.Name {
		t.Fatalf("channel was not updated: name: %s", channel.Name)
	}
}

func TestChannelUpdateThread(t *testing.T) {
	tracker := createTestState(TrackerConfig{
		BotMemberID: initialtestBotID,
	})
	channel := tracker.GetGuild(initialTestGuildID).GetChannel(initialTestThreadID)
	if channel == nil {
		t.Fatal("thread not found")
	}

	updt := createTestThread(1, initialTestThreadID, nil)
	updt.Name = "this is a new name!"

	tracker.HandleEvent(testSession, &discordgo.ChannelUpdate{
		Channel: updt,
	})

	channel = tracker.GetGuild(initialTestGuildID).GetChannel(initialTestThreadID)
	if channel == nil {
		t.Fatal("thread not found")
	}

	if channel.Name != updt.Name {
		t.Fatalf("thread was not updated: name: %s", channel.Name)
	}
}

func TestRoleUpdate(t *testing.T) {
	tracker := createTestState(TrackerConfig{
		BotMemberID: initialtestBotID,
	})
	role := tracker.GetGuild(initialTestGuildID).GetRole(initialTestRoleID)
	if role == nil {
		t.Fatal("role not found")
	}

	updt := &discordgo.GuildRole{
		Role: &discordgo.Role{
			ID:   initialTestRoleID,
			Name: "new role name!",
		},
		GuildID: initialTestGuildID,
	}

	tracker.HandleEvent(testSession, &discordgo.GuildRoleUpdate{
		GuildRole: updt,
	})

	role = tracker.GetGuild(initialTestGuildID).GetRole(initialTestRoleID)
	if role == nil {
		t.Fatal("role not found")
	}

	if role.Name != updt.Role.Name {
		t.Fatalf("role was not updated: name: %s", role.Name)
	}
}

func createTestStateThreadEvents(conf TrackerConfig) *InMemoryTracker {
	state := createTestState(conf)
	state.HandleEvent(testSession, &discordgo.ThreadListSync{
		GuildID:    initialTestGuildID,
		ChannelIDs: []int64{initialTestChannelID},
		Members: []*discordgo.ThreadMember{
			{
				ID:            initialTestThreadID,
				UserID:        initialtestBotID,
				JoinTimestamp: discordgo.Timestamp(time.Now().Format(time.RFC3339)),
				Flags:         1 << 1,
			},
		},
		Threads: []*discordgo.Channel{
			createTestThread(initialTestGuildID, initialTestThreadID, nil),
		},
	})

	return state
}

func TestThreadEvents(t *testing.T) {
	tracker := createTestStateThreadEvents(TrackerConfig{
		BotMemberID: initialtestBotID,
	})

	thread := tracker.GetGuild(initialTestGuildID).GetChannel(initialTestThreadID)
	if thread == nil {
		t.Fatal("thread in TestThreadEvents is nil")
	}

	if thread.Member == nil {
		t.Fatal("thread member not set")
	}

	tracker.HandleEvent(testSession, &discordgo.ThreadListSync{
		GuildID:    initialTestGuildID,
		ChannelIDs: []int64{initialTestChannelID},
	})

	shard := tracker.getGuildShard(initialTestGuildID)
	if shard == nil {
		t.Fatal("shard is nil")
	}

	thread = tracker.GetGuild(initialTestGuildID).GetChannel(initialTestThreadID)
	if thread != nil {
		t.Fatal("thread was not removed from state")
	}

	_, ok := shard.threadsGuildID[initialTestThreadID]
	if ok {
		t.Fatal("shard.threadsGuildID not removed on handleThreadListSync")
	}

	tracker.HandleEvent(testSession, &discordgo.ThreadListSync{
		GuildID: initialTestGuildID,
		Threads: []*discordgo.Channel{
			{
				ID:      secondTesThreadID,
				GuildID: initialTestGuildID,
			},
		},
	})

	_, ok = shard.threadsGuildID[secondTesThreadID]
	if !ok {
		t.Fatal("shard.threadsGuildID not set on handleThreadListSync")
	}

	tracker.HandleEvent(testSession, &discordgo.ThreadMemberUpdate{
		ThreadMember: &discordgo.ThreadMember{
			ID:            secondTesThreadID,
			UserID:        initialtestBotID,
			JoinTimestamp: discordgo.Timestamp(time.Now().Format(time.RFC3339)),
			Flags:         1 << 2, // We change the flag just to make sure it went through
		},
	})

	memberFlag := tracker.GetGuild(initialTestGuildID).GetChannel(secondTesThreadID).Member.Flags
	if memberFlag != 1<<2 {
		t.Fatal("member not updated on handleThreadMemberUpdate")
	}

	tracker.HandleEvent(testSession, &discordgo.ThreadCreate{
		Channel: createTestThread(initialTestGuildID, secondTesThreadID, nil),
	})

	tracker.HandleEvent(testSession, &discordgo.ThreadMembersUpdate{
		ID:      secondTesThreadID,
		GuildID: initialTestGuildID,
		AddedMembers: []*discordgo.ThreadMember{
			{
				ID:            secondTesThreadID,
				UserID:        initialtestBotID,
				JoinTimestamp: discordgo.Timestamp(time.Now().Format(time.RFC3339)),
				Flags:         1 << 3, // We change the flag just to make sure it went through
			},
		},
	})

	memberFlag = tracker.GetGuild(initialTestGuildID).GetChannel(secondTesThreadID).Member.Flags
	if memberFlag != 1<<3 {
		t.Fatal("member not updated on handleThreadMembersUpdate")
	}

	tracker.HandleEvent(testSession, &discordgo.ThreadMembersUpdate{
		ID:                secondTesThreadID,
		GuildID:           initialTestGuildID,
		RemovedMembersIDs: []int64{initialtestBotID},
	})

	member := tracker.GetGuild(initialTestGuildID).GetChannel(secondTesThreadID).Member
	if member != nil {
		t.Fatal("member not set to nil")
	}

	tracker.HandleEvent(testSession, &discordgo.ThreadListSync{
		GuildID: initialTestGuildID,
		Threads: []*discordgo.Channel{
			{
				ID:      initialTestThreadID,
				GuildID: initialTestGuildID,
			},
		},
	})

	tracker.HandleEvent(testSession, &discordgo.ThreadDelete{
		Channel: &discordgo.Channel{
			ID:      initialTestThreadID,
			GuildID: initialTestGuildID,
		},
	})

	_, ok = shard.threadsGuildID[initialTestThreadID]
	if ok {
		t.Fatal("shard.threadsGuildID not removed on ThreadDelete")
	}

	thread = tracker.GetGuild(initialTestGuildID).GetChannel(initialTestThreadID)
	if thread != nil {
		t.Fatal("thread not removed on ThreadDelete")
	}
}
