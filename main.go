package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/multiplay/go-ts3"
	"github.com/orsetii/discord2ts3/data"
	sq "github.com/paralin/ts3-go/serverquery"
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
	//Client is the global sq client interface
	Client *sq.ServerQueryAPI
	//TsStateInfo is a channel used to monitor who is in discord when called.
	TsStateInfo = make(chan string, 10)
	// TS command to send message. Able to specify message but user needs to be done earlier in the client.
	MsgCmd = ts3.NewCmd("sendtextmessage targetmode=2 target=1").WithArgs(ts3.NewArg("msg", "Test1")) // TODO add
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

// TODO add so can send as x user, taking in user as an arg.
func tsSend(msg string) error {
	client, err := ts3.NewClient(data.Addr) // @TODO PORT ALL TS FUNCTIONALITY TO ts3 package from multiplay
	checkErr(err)
	client.Use(1)
	client.Login(tsUser, tsPass)
	sendTextMessage := ts3.NewCmd("sendtextmessage targetmode=2 target=1").WithArgs(ts3.NewArg("msg", msg))
	ret, err := client.ExecCmd(sendTextMessage)
	fmt.Printf("\nALERT:\n%#v\n%#v\n", ret, err)
	client.Logout()
	return err
}

func tsConn() (*sq.ServerQueryAPI, error) {
	return sq.Dial(data.Addr)
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
					if strings.Contains(clientSummary.Nickname, "serveradmin") {
						continue
					}
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

	/* // Init Second sq Client for messages
	client, err := ts3.NewClient(data.Addr) // @TODO PORT ALL TS FUNCTIONALITY TO ts3 package from multiplay
	checkErr(err)
	client.Use(1)
	client.Login(tsUser, tsPass)*/
	// Register the discSend func as a callback for MessageCreate events.
	dg.AddHandler(discMsgHandle)

	tsSend("Test123")
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

//TODO CHANGE THIS TO BE A FUNCTION TO RECEIVE MESSAGES
// func (c *Client) RegisterChannel(id uint) error {
// 	_, err := c.ExecCmd(NewCmd("servernotifyregister").WithArgs(
// 		NewArg("event", ChannelEvents),
// 		NewArg("id", id),
// 	))
// 	return err
// }

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
		//_, err := Client.ExecuteCommand(Ctx, &sq.UseCommand())
		//err := Client.SendTextMessage(Ctx, 1, 0, "hello")
		//a := new(sq.ServerQueryReadWriter)
		//checkErr(err)
		msg := m.Author.Username + ": " + m.Content
		tsSend(msg)
		return
	}

}

// UseCommand is the use command.
type UseCommand struct {
	// Port is the server to use.
	Port int `serverquery:"port"`
}

// GetResponseType returns an instance of the response type.
func (c *UseCommand) GetResponseType() interface{} {
	return nil
}

// GetCommandName returns the name of the command.
func (c *UseCommand) GetCommandName() string {
	return "use"
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
