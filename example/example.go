package main

import (
	"fmt"
	"github.com/jonas747/discordgo"
	"github.com/jonas747/jdshardmanager"
	"github.com/DevKurka/dstate"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

func main() {
	state := dstate.NewState()

	state.MaxChannelMessages = 1000
	state.MaxMessageAge = time.Hour
	state.ThrowAwayDMMessages = true
	state.TrackPrivateChannels = false
	state.CacheExpirey = time.Minute * 10

	sm := dshardmanager.New(os.Getenv("DG_TOKEN"))
	sm.AddHandler(state.HandleEvent)

	sm.SessionFunc = func(token string) (*discordgo.Session, error) {
		s, err := discordgo.New(token)
		if err != nil {
			return nil, err
		}

		s.StateEnabled = false
		s.SyncEvents = true
		s.LogLevel = discordgo.LogInformational

		return s, nil
	}

	go dumpStatsLoop(state)

	err := sm.Start()
	if err != nil {
		panic(err)
	}

	fmt.Println("Running...")

	select {}
}

func dumpStatsLoop(state *dstate.State) {
	t := time.NewTicker(time.Second)

	lastProf := time.Now()

	var memstats runtime.MemStats
	for {
		<-t.C

		runtime.ReadMemStats(&memstats)

		gcSecs := memstats.LastGC / 1000000000
		tGC := time.Unix(int64(gcSecs), 0)

		log.Printf("Heap Allocs: %10dMB (n: %7d) - last gc: %5.0fs, Stack allocs: %5dKB", memstats.HeapAlloc/1000000, memstats.Mallocs-memstats.Frees, time.Since(tGC).Seconds(), memstats.StackInuse/1000)

		if time.Since(lastProf) > time.Minute*1 {
			// if time.Since(lastProf) > time.Second*10 {
			heapProfile(fmt.Sprintf("%d.mprof", time.Now().Unix()))
			lastProf = time.Now()
			log.Println("Dumped memory profile")

			totalGuilds, totalMembers, totalChannels, totalMessages, totalRoles := StateInfo(state)
			log.Printf("g:%5d - m:%7d - c:%7d - msg:%7d - r:%7d", totalGuilds, totalMembers, totalChannels, totalMessages, totalRoles)
		}
	}
}

func heapProfile(name string) {
	f, err := os.Create(name)
	if err != nil {
		log.Println("failed creating profile file: " + err.Error())
		return
	}

	err = pprof.WriteHeapProfile(f)
	f.Close()
	if err != nil {
		log.Println("failed writing heap profile: " + err.Error())
	}
}

func StateInfo(state *dstate.State) (totalGuilds, totalMembers, totalChannels, totalMessages, totalRoles int) {
	state.RLock()
	totalChannels = len(state.Channels)
	totalGuilds = len(state.Guilds)
	gCop := state.GuildsSlice(false)
	state.RUnlock()

	for _, g := range gCop {
		g.RLock()

		totalMembers += len(g.Members)

		for _, cState := range g.Channels {
			totalMessages += len(cState.Messages)
		}

		totalRoles += len(g.Guild.Roles)

		g.RUnlock()
	}

	return
}
