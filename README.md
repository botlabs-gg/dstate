# dstate

!! This only works with my discordgo fork !!

dstate is an alternative state tracker to the standard one in discordgo.

It's a bit more advanced but offer more features and it's easier to avoid race conditions with.

Example:

Retrieving a channel, and getting the name without data races
```go
// Standard state in discrodgo

// call channel, state is rlocked inside
channel := state.Channel(id)
// We have to rlock the whole state to get the name
state.RLock()
name := channel.Name
state.RUnlock()



// dstate

// call channel, state is rlocked inside if lock is set to true
channelState := state.Channel(true, id)
// Instead of locking the whole state, we either lock just the channel if it's a private channel, or the parent guild
channelState.Owner.RLock()
name := channelstate.Channel.name
channelState.Owner.RUnlock()

// can also create a copy, which after creation you can access and modify fields without worrying about data races as it's a copy
channelCopy := channelState.Copy(lock, deep /*also copy perm overwrites*/)

// Some data can be accessed safely without locking as they are never mutated:
channelState.ID()
channelState.Type()
channelState.IsPrivate()
channelState.Recipient()
```

Differences:

 - Per guild rw mutex
     + So you don't need to lock the whole state if you want to avoid race conditions
 - Optionally keep deleted messages in state (with a flag on them set if deleted)
 - Presence tracking
 - Optionally remove offline members from state (if your're on limited memory)
 - Set a max message age to only keep messages up untill a certain age in the state
