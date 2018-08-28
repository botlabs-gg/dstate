package main

import (
	"fmt"
	"github.com/jonas747/discordgo"
	"github.com/jonas747/dutil/dstate"
	"os"
)

func main() {
	session, err := discordgo.New(os.Getenv("DG_TOKEN"))
	if err != nil {
		panic(err)
	}

	state := dstate.NewState()
	session.AddHandler(state.HandleEvent)

	err = session.Open()
	if err != nil {
		panic(err)
	}

	fmt.Println("Running...")

	select {}
}
