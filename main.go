package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"strconv"
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
	// MsgCmd is a TS command to send message. Able to specify message but user needs to be done earlier in the client.
	MsgCmd = ts3.NewCmd("sendtextmessage targetmode=2 target=1").WithArgs(ts3.NewArg("msg", "Test_in_global_var"))
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

		tsInit(discord) // @TODO change to goroutine
		wg.Add(1)
		checkErr(err)

		wg.Wait()
	}

	app.RunAndExitOnError()
}

// WORKING!
func tsSend(user, pass, msg string) error {
	client, err := ts3.NewClient(data.Addr) // @TODO PORT ALL TS FUNCTIONALITY TO ts3 package from multiplay
	checkErr(err)
	client.Use(1)
	client.Login(user, pass)
	sendTextMessage := ts3.NewCmd("sendtextmessage targetmode=2 target=1").WithArgs(ts3.NewArg("msg", msg))
	ret, err := client.ExecCmd(sendTextMessage)
	fmt.Printf("\nALERT:\n%v\n%v\n", ret, err)
	client.Logout()
	return err
}

func tsConn() (*sq.ServerQueryAPI, error) {
	return sq.Dial(data.Addr)
}

func tsInit(dg *discordgo.Session) {

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
			// Clear channel and start
			<-TsStateInfo
			var retString string = "```"
			retString += fmt.Sprintf("\n %s\t\t\t\t\t\t%s\t", "Name", "Mute Status")
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

					var fmtNick = clientInfo.Nickname
					if nickLen := len(fmtNick); nickLen > 15 {
						fmtNick = fmtNick[:12] + "..."
					}

					var isMuted string
					// Muted status checker
					switch {
					case clientInfo.OutputMuted:
						isMuted = "🔇"
					case clientInfo.InputMuted:
						isMuted = "Mic"
					default:
						if clientInfo.IsTalker {
							isMuted = "Speaking"
						} else {
							isMuted = "Unmuted"
						}

					}
					var row string // Next row to print
					row += fmt.Sprintf(" %-16s", fmtNick)
					row += fmt.Sprintf("\t\t\t%s\n", isMuted)
					retString += row
				}
				retString += "```"
				fmt.Printf("%s", retString)
				TsStateInfo <- retString
			}
		}
	}()
	fmt.Printf("Waiting for events.\n")
	//if err := Client.ServerNotifyRegisterAll(Ctx); err != nil {
	//	checkErr(err)
	//}
	err = Client.ServerNotifyRegister(Ctx, "textchannel", 1)
	log.Println(err)
	events := Client.Events()
	for event := range events {
		v := reflect.ValueOf(event).Elem()

		// Now get Message and user id of sender
		tsMsg := v.FieldByIndex([]int{1})
		findSender := v.FieldByIndex([]int{5})
		fmt.Println(findSender.String())
		tsSender := data.TsToName[findSender.String()]

		err := tsToDiscSend(dg, tsSender, tsMsg.String())
		if err != nil {
			log.Println(err)
			continue
		}
		fmt.Printf("reflect: %v\n", v.FieldByIndex([]int{1}))
		fmt.Printf("message: %v\n", event) //  Example output: event: &serverquery.TextMessageReceived{TargetMode:2, Message:"a", TargetID:0, InvokerID:253, InvokerName:"orseti", InvokerUID:"Kb9cMhGIapfej2uj0r6GrSF64aQ="}
	}
}

func tsToDiscSend(dg *discordgo.Session, name, msg string) error {
	// Using general-text2 as channel for now
	discChan := "665962482694750228"
	for _, value := range data.DiscToName {
		if value == name {
			dg.ChannelMessageSend(discChan, "\n"+name+": "+msg)
			return nil
		}
		// If we find someone in the discordID to name table with the same name passed as Arg:

	}
	return fmt.Errorf("%s not found in DiscordIDDatabase", name)
}

func tsPoke(user, pass, toPoke, msg string) error { // TODO cut out the !poke and the name
	client, err := ts3.NewClient(data.Addr) // @TODO PORT ALL TS FUNCTIONALITY TO ts3 package from multiplay
	checkErr(err)
	client.Use(1)
	client.Login(user, pass)
	// Get online clients via ClientList()
	// Find Client in ClientDBList
	// Get UID of client from that DBList
	//
	clientList, err := client.Server.ClientList()
	clientDBList, err := client.Server.ClientDBList()
	if err != nil {
		return err
	}
	ChanToPoke := make(chan int, 1)

	for _, person := range clientDBList {
		if data.TsToName[person.UniqueIdentifier] == toPoke { // If the user passed to us is found in the database:
			for _, personClientInfo := range clientList {
				if person.Nickname == personClientInfo.Nickname { // If we DO have a match between dbinfo and clientinfo
					getCLID := ts3.NewCmd("clientgetids ").WithArgs(ts3.NewArg("cluid", person.UniqueIdentifier))
					ret, err := client.ExecCmd(getCLID)
					if err != nil {
						return err
					}
					startOfClid := len(person.UniqueIdentifier) + 12
					ToPokeID, err := strconv.Atoi(ret[0][startOfClid : startOfClid+3])
					if err != nil {
						return err
					}
					ChanToPoke <- ToPokeID
					break
				}
			}
		}
	}
	msg = user[1:] + " poked you from discord: " + msg
	sendPoke := ts3.NewCmd("clientpoke ").WithArgs(ts3.NewArg("clid", <-ChanToPoke), ts3.NewArg("msg", msg))
	ret, err := client.ExecCmd(sendPoke)
	fmt.Printf("\nPOKED:\n%v\n%v\n", ret, err)
	client.Logout()
	return err
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
	if strings.Contains(m.Content, "!poke") {
		senderName := "d" + data.DiscToName[m.Author.ID]
		senderPass := data.SQData[senderName]
		msg := strings.Split(m.Content, " ")

		tsPoke(senderName, senderPass, msg[1], msg[2])
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
		senderName := data.DiscToName[m.Author.ID]
		senderName = "d" + senderName
		senderPass := data.SQData[senderName]
		tsSend(senderName, senderPass, m.Content)
		return
	}

}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
