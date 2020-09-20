package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/orsetii/discord2ts3/data"
	ts3 "github.com/paralin/ts3-go/serverquery"
	"github.com/urfave/cli"
)

const (
	tsUser = "serveradmin"
	tsPass = "s0h6sfj1"
)

var (
	wg sync.WaitGroup
	//Ctx is the context for whole package
	Ctx context.Context
	//Client is the global ts3 client interface
	Client *ts3.ServerQueryAPI
	//TsStateInfo is a channel used to monitor who is in discord when called.
	TsStateInfo = make(chan string, 10)
)

func main() {

	app := cli.NewApp()
	app.Name = "discord2ts3"
	app.Action = func(c *cli.Context) {
		discord, err := discordgo.New("Bot " + data.DiscAuthToken)
		checkErr(err)

		wg.Add(1)
		go discInit(discord)
		checkErr(err)
		wg.Add(1)

		tsInit()
		wg.Add(1)
		checkErr(err)

		wg.Wait()
	}

	app.RunAndExitOnError()
}
func tsConn() (*ts3.ServerQueryAPI, error) {
	return ts3.Dial(data.Addr)
}

func tsInit() {

	Ctx := context.Background()
	Client, err := tsConn()
	if err != nil {
		if err.Error() == "EOF" {
			log.Fatalln("Teamspeak Admin Server Disconnected the client immediately.")
		} else {
			checkErr(err)
		}
	}

	go Client.Run(Ctx)
	if err := Client.UseServer(Ctx, 9987); err != nil {
		checkErr(err)
	}
	if err := Client.Login(Ctx, tsUser, tsPass); err != nil {
		checkErr(err)
	}

	channelList, err := Client.GetChannelList(Ctx)
	if err != nil {
		checkErr(err)
	}
	dat, _ := json.Marshal(channelList)
	fmt.Printf("channel list: %#v\n", string(dat))
	for _, channelSummary := range channelList {
		channelInfo, err := Client.GetChannelInfo(Ctx, channelSummary.Id)
		if err != nil {
			checkErr(err)
		}
		dat, _ = json.Marshal(channelInfo)
		fmt.Printf("channel [%d]: %#v\n", channelSummary.Id, string(dat))
	}
	go func() {
		for {
			time.Sleep(time.Second)
			// If Channel empty
			var retString string = "```"
			retString += fmt.Sprintf("\n %s\t\t\t\t\t\t%s\t", "Name", "AFK")
			retString += fmt.Sprintf("\n %s\t\t\t\t\t\t%s\t\n", "----", "----")

			if len(TsStateInfo) == 0 {
				clientList, err := Client.GetClientList(Ctx)
				if err != nil {
					continue
				}

				for _, clientSummary := range clientList {
					clientInfo, err := Client.GetClientInfo(Ctx, clientSummary.Id)
					if err != nil {
						continue
					}
					var isIdle string = "No"
					if clientInfo.ClientIdleTime > 30000 {
						isIdle = "Yes"
					}
					var fmtNick = clientInfo.Nickname
					if nickLen := len(fmtNick); nickLen > 15 {
						fmtNick = fmtNick[:12] + "..."
					}
					var row string // Next row to print
					row += fmt.Sprintf(" %-16s", fmtNick)
					row += fmt.Sprintf("\t\t\t%s\n", isIdle)
					retString += row
				}
				retString += "```"
				fmt.Printf("%s", retString)
				TsStateInfo <- retString
			}
		}
	}()
	fmt.Printf("Waiting for events.\n")
	if err := Client.ServerNotifyRegisterAll(Ctx); err != nil {
		checkErr(err)
	}

	events := Client.Events()
	for event := range events {
		fmt.Printf("event: %#v\n", event)
	}
}

func discInit(dg *discordgo.Session) {
	// Register the discSend func as a callback for MessageCreate events.
	dg.AddHandler(discMsgHandle)

	// In this example, we only care about receiving message events.
	dg.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildMessages)

	// Open a websocket connection to Discord and begin listening.
	err := dg.Open()
	if err != nil {
		fmt.Println("error opening connection. Error:", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	// Cleanly close down the Discord session.
	dg.Close()
	wg.Done()
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func discMsgHandle(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}
	switch m.Content {
	// If the message is "ping" reply with "Pong!"
	case "ping":
		s.ChannelMessageSend(m.ChannelID, "Pong!")

		// If the message is "pong" reply with "Ping!"
	case "pong":
		s.ChannelMessageSend(m.ChannelID, "Ping!")
	case "tsinfo":
		fallthrough
	case "!tsinfo":
		s.ChannelMessageSend(m.ChannelID, <-TsStateInfo)
	default:
		// TODO here is ALL messages that dont have a command associated with them.
		err := Client.SendTextMessage(Ctx, 2, 1, "hello")
		checkErr(err)
		return
	}

}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
