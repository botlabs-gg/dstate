package inmemorytracker

import (
	"github.com/jonas747/discordgo"
	"github.com/jonas747/dstate/v3"
)

var _ dstate.StateTracker = (*InMemoryTracker)(nil)

func (tracker *InMemoryTracker) GetGuild(guildID int64) *dstate.GuildSet {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	set, ok := shard.guilds[guildID]
	if !ok {
		return nil
	}

	return &dstate.GuildSet{
		GuildState:  *set.Guild,
		Channels:    set.Channels,
		Roles:       set.Roles,
		Emojis:      set.Emojis,
		VoiceStates: set.VoiceStates,
	}
}

func (tracker *InMemoryTracker) GetMember(guildID int64, memberID int64) *dstate.MemberState {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	return shard.getMemberLocked(guildID, memberID)
}

func (shard *ShardTracker) getMemberLocked(guildID int64, memberID int64) *dstate.MemberState {
	if ml, ok := shard.members[guildID]; ok {
		for _, v := range ml {
			if v.User.ID == memberID {
				return v
			}
		}
	}

	return nil
}

func (tracker *InMemoryTracker) GetMemberPermissions(guildID int64, channelID int64, memberID int64) (perms int64, ok bool) {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	member := shard.getMemberLocked(guildID, memberID)
	if member == nil {
		return 0, false
	}

	return tracker.getRolePermisisonsLocked(shard, guildID, channelID, memberID, member.Roles)
}

func (tracker *InMemoryTracker) GetRolePermisisons(guildID int64, channelID int64, memberID int64, roles []int64) (perms int64, ok bool) {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	return tracker.getRolePermisisonsLocked(shard, guildID, channelID, memberID, roles)
}

func (tracker *InMemoryTracker) getRolePermisisonsLocked(shard *ShardTracker, guildID int64, channelID int64, memberID int64, roles []int64) (perms int64, ok bool) {
	ok = true

	guild, ok := shard.guilds[guildID]
	if !ok {
		return 0, false
	}

	var overwrites []discordgo.PermissionOverwrite

	if channel := guild.channel(channelID); channel != nil {
		overwrites = channel.PermissionOverwrites
	} else if channelID != 0 {
		// we still continue as far as we can with the calculations even though we can't apply channel permissions
		ok = false
	}

	perms = dstate.CalculatePermissions(guild.Guild, guild.Roles, overwrites, memberID, roles)
	return perms, ok
}

// func (tracker *InMemoryTracker) GetGuild(guildID int64) *dstate.GuildState {
// 	shard := tracker.getGuildShard(guildID)
// 	shard.mu.RLock()
// 	defer shard.mu.RUnlock()

// 	if guild, ok := shard.guilds[guildID]; ok {
// 		return guild.Guild
// 	}

// 	return nil
// }

func (tracker *InMemoryTracker) GetChannel(guildID int64, channelID int64) *dstate.ChannelState {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	if guild, ok := shard.guilds[guildID]; ok {
		for _, v := range guild.Channels {
			if v.ID == channelID {
				return v
			}
		}
	}

	return nil
}

func (tracker *InMemoryTracker) GetRole(guildID int64, roleID int64) *discordgo.Role {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	if guild, ok := shard.guilds[guildID]; ok {
		for _, v := range guild.Roles {
			if v.ID == roleID {
				return v
			}
		}
	}

	return nil
}

func (tracker *InMemoryTracker) GetEmoji(guildID int64, emojiID int64) *discordgo.Emoji {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	if guild, ok := shard.guilds[guildID]; ok {
		for _, v := range guild.Emojis {
			if v.ID == emojiID {
				return v
			}
		}
	}

	return nil
}

func (tracker *InMemoryTracker) getGuildShard(guildID int64) *ShardTracker {
	shardID := int((guildID >> 22) % tracker.totalShards)
	return tracker.shards[shardID]
}

func (tracker *InMemoryTracker) getShard(shardID int64) *ShardTracker {
	return tracker.shards[shardID]
}

func (tracker *InMemoryTracker) cloneMembers(guildID int64) []*dstate.MemberState {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	membersCop := make([]*dstate.MemberState, len(shard.members[guildID]))
	if len(membersCop) < 1 {
		return nil
	}

	copy(membersCop, shard.members[guildID])
	return membersCop
}

// this IterateMembers implementation is very simple, it makes a full copy of the member slice and calls f in one chunk
func (tracker *InMemoryTracker) IterateMembers(guildID int64, f func(chunk []*dstate.MemberState) bool) {
	members := tracker.cloneMembers(guildID)
	if len(members) < 1 {
		return // nothing to do
	}

	f(members)
}

func (tracker *InMemoryTracker) cloneMessages(guildID int64, channelID int64) []*dstate.MessageState {
	shard := tracker.getGuildShard(guildID)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	messages := shard.messages[channelID]
	if messages == nil {
		return nil
	}

	messagesCop := make([]*dstate.MessageState, shard.messages[channelID].Len())
	if len(messagesCop) < 1 {
		return nil
	}

	for e := messages.Front(); e != nil; e = e.Next() {
		messagesCop = append(messagesCop, e.Value.(*dstate.MessageState))
	}

	return messagesCop
}

// this IterateMessages implementation is very simple, it makes a full copy of the messages slice and calls f in one chunk
// func (tracker *InMemoryTracker) IterateMessages(guildID int64, channelID int64, f func(chunk []*dstate.MessageState) bool) {
// 	messages := tracker.cloneMessages(guildID, channelID)
// 	if len(messages) < 1 {
// 		return // nothing to do
// 	}

// 	f(messages)
// }

func (tracker *InMemoryTracker) GetMessages(guildID int64, channelID int64) []*dstate.MessageState {
	return tracker.cloneMessages(guildID, channelID)
}

func (tracker *InMemoryTracker) GetShardGuilds(shardID int64) []*dstate.GuildSet {
	shard := tracker.getShard(shardID)
	if shard == nil {
		return nil
	}

	shard.mu.RLock()
	defer shard.mu.RUnlock()

	gCop := make([]*dstate.GuildSet, len(shard.guilds))
	for i, v := range shard.guilds {
		gCop[i] = &dstate.GuildSet{
			GuildState:  *v.Guild,
			Channels:    v.Channels,
			Roles:       v.Roles,
			Emojis:      v.Emojis,
			VoiceStates: v.VoiceStates,
		}
	}

	return gCop
}
