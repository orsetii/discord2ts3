package main

import (
	"context"
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
	TsStateInfo = make(chan string, 1)
	// MsgCmd is a TS command to send message. Able to specify message but user needs to be done earlier in the client.
	MsgCmd = ts3.NewCmd("sendtextmessage targetmode=2 target=1").WithArgs(ts3.NewArg("msg", "Test_in_global_var"))
)

const (
	// #general-text2 = 665962482694750228
	// #botspam = 716760837347606568
	discChannel = "665962482694750228"
)

func main() {
	app := cli.NewApp()
	app.Name = "discord2ts3"
	app.Action = func(c *cli.Context) {
		discord, err := discordgo.New("Bot " + data.DiscAuthToken)
		checkErr(err)
		go discInit(discord)
		wg.Add(1)

		go tsInit(discord)
		wg.Add(1)

		wg.Wait()
	}

	app.RunAndExitOnError()
}

// NOT WORKING!
func discToTsSend(user, pass, msg string) error {
	client, err := ts3.NewClient(data.Addr)
	checkErr(err)
	client.Use(1)
	client.Login(user, pass)
	log.Printf("Relaying message from Discord to Teamspeak. Message Content: %s", msg)
	sendTextMessage := ts3.NewCmd("sendtextmessage targetmode=2 target=1").WithArgs(ts3.NewArg("msg", msg))
	resp, err := client.ExecCmd(sendTextMessage)
	fmt.Printf("RESPONSE OF TS TXTMSG COMMAND: %s\nERROR: %v\n", resp, err)
	client.Logout()
	client.Close()
	return err
}

func tsConn() (*sq.ServerQueryAPI, error) {
	return sq.Dial(data.Addr)
}

func tsInit(dg *discordgo.Session) {
	Ctx := context.Background()
	Client, err := tsConn()
	defer Client.Close()

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
	fmt.Println("Waiting...")
	//if err := Client.ServerNotifyRegisterAll(Ctx); err != nil {
	//	checkErr(err)
	//}
	err = Client.ServerNotifyRegister(Ctx, "textchannel", 1)
	if err != nil {
		log.Println(err)
	}
	events := Client.Events()
	go func() {
		for {
			time.Sleep(time.Second)
			_, err = Client.GetChannelInfo(Ctx, 1)
			if err != nil {
				fmt.Println("Error in polling server connection:115:main.go")
			} else {
				fmt.Println("Valid Conn")
			}
		}
	}()
	for event := range events {
		v := reflect.ValueOf(event).Elem()

		// Now get Message and user id of sender
		tsMsg := v.FieldByIndex([]int{1})
		findSender := v.FieldByIndex([]int{5})

		tsSender := data.TsToName[findSender.String()]
		if tsSender == "" {
			continue
		}
		err := tsToDiscSend(dg, tsSender, tsMsg.String())
		if err != nil {
			log.Printf("ERROR attempted to relay ts message to discord: \"%s\"", err)
			continue
		}
		fmt.Printf("Message: %s\n", v.FieldByIndex([]int{1}))
		time.Sleep(time.Second)
	}
}

func discTsInfo() string {
	newCtx := context.Background()
	Client, err := tsConn()
	if err != nil {
		if err.Error() == "EOF" {
			log.Fatalln("Teamspeak Admin Server Disconnected the client immediately.")
		} else {
			checkErr(err)
		}
	}

	go Client.Run(newCtx)
	if err := Client.UseServer(newCtx, 9987); err != nil {
		checkErr(err)
	}
	if err := Client.Login(newCtx, "orseti", "oyQPiIfv"); err != nil {
		checkErr(err)
	}
	// Clear channel and start
	var retString string = "```"
	retString += fmt.Sprintf("\n %s\t\t\t\t\t\t%s\t", "Name", "Mute Status")
	retString += fmt.Sprintf("\n %s\t\t\t\t\t\t%s\t\n", "----", "----")

	clientList, err := Client.GetClientList(newCtx)
	if err != nil {
		log.Println()
	}

	for _, clientSummary := range clientList {
		if strings.Contains(clientSummary.Nickname, "serveradmin") || clientSummary.Nickname[0] == 'd' || strings.ContainsAny(clientSummary.Nickname[2:], "123456789") {
			continue
		}
		clientInfo, err := Client.GetClientInfo(newCtx, clientSummary.Id)
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
			isMuted = "🎤"
		default:
			if clientInfo.IsTalker {
				isMuted = "🔊 Speaking now"
			} else {
				isMuted = "🔊"
			}

		}
		var row string // Next row to print
		row += fmt.Sprintf(" %-16s", fmtNick)
		row += fmt.Sprintf("\t\t\t%s\n", isMuted)
		retString += row
	}
	retString += "```"
	fmt.Printf("%s", retString)
	//TsStateInfo <- retString
	return retString
}

func tsToDiscSend(dg *discordgo.Session, name, msg string) error {
	// Using general-text2 as channel for now
	defer fmt.Printf("Message Sent from Ts to Discord: %s", msg)
	for _, value := range data.DiscToName {
		if value == name {
			if strings.Contains(msg, "@") {
				var endSplitMsg []string
				splitMsg := strings.Split(msg, " ")
				for _, V := range splitMsg {
					if strings.Contains(V, "@") {
						firstChar := strings.ToUpper(string(V[1]))
						V = "@" + firstChar + V[2:]
						for id, discName := range data.DiscToName {
							if discName == V[1:] {
								V = "<@" + id + ">"
							}
						}
					}
					endSplitMsg = append(endSplitMsg, V)
				}
				msg = strings.Join(endSplitMsg, " ")
			}
			if strings.Contains(msg, "[URL]") {
				msg = strings.Replace(msg, "[URL]", "", 1)
				msg = strings.Replace(msg, "[/URL]", "", 1)
			}
			dg.ChannelMessageSend(discChannel, "\n"+name+": "+msg)
			return nil
		}
		// If we find someone in the discordID to name table with the same name passed as Arg:

	}
	return fmt.Errorf("%s not found in DiscordIDDatabase", name)
}

func tsPoke(user, pass, toPoke, msg string) error {
	client, err := ts3.NewClient(data.Addr)
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
					recList, err := client.ExecCmd(getCLID)
					if err != nil {
						return err
					}
					ret := recList[0]
					startOfClid := len(person.UniqueIdentifier) + 12
					ToPokeID, err := strconv.Atoi(ret[startOfClid : startOfClid+3])
					if ToPokeID == 0 {
						// Remove all including 'clid=' 5 chars
						preToPokeID := strings.Split(ret, " ")[1]
						ToPokeID, err = strconv.Atoi(preToPokeID[5:])
						if err != nil {
							log.Printf("Errored Finding ID at main.go:253\nError msg: %s", err)
						}
						log.Printf("Found user %s with ID %d", personClientInfo.Nickname, ToPokeID)
					}
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
	_, err = client.ExecCmd(sendPoke)
	fmt.Printf("User %s sent poke.\n", user[1:])
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
	if m.Author.ID == s.State.User.ID {
		return
	}
	if strings.Contains(m.Content, "!poke") {
		senderName := "d" + data.DiscToName[m.Author.ID]
		senderPass := data.SQData[senderName]
		msg := strings.Split(m.Content, " ")
		if len(msg) < 3 {
			tsPoke(senderName, senderPass, msg[1], "")
			return
		}
		log.Printf("Poking User: %s from User: %s...", msg[1], senderName[1:])
		if err := tsPoke(senderName, senderPass, msg[1], msg[2]); err != nil {
			log.Println(err)
			return
		}
		log.Println("No error reported.")
		return
	}
	switch m.Content {
	// If the message is "ping" reply with "Pong!"
	case "tsinfo":
		fallthrough
	case "!tsinfo":
		msg := discTsInfo()
		s.ChannelMessageSend(m.ChannelID, msg)
	default:
		senderName := data.DiscToName[m.Author.ID]
		senderName = "d" + senderName
		senderPass := data.SQData[senderName]
		err := discToTsSend(senderName, senderPass, m.Content)
		if err != nil {
			fmt.Printf("ERROR IN SENDING TS MESSAGE: %s", err)
		}
		return
	}
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
