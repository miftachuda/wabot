package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func RunCommand(s string, stdin io.Writer) {
	io.WriteString(stdin, s)
}

type MyClient struct {
	worker         io.Writer
	WAClient       *whatsmeow.Client
	subprocess     *exec.Cmd
	channel        chan types.JID
	eventHandlerID uint32
}

func (mycli *MyClient) register() {
	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)
}

func (mycli *MyClient) myEventHandler(evt interface{}) {
	// Handle event and access mycli.WAClient

	switch v := evt.(type) {
	case *events.Message:
		fmt.Println("Received a message!", v.Message.GetConversation())
		if v.Message.GetConversation() == "halo" {
			mycli.WAClient.SendMessage(context.Background(), v.Info.Sender, "", &waProto.Message{
				Conversation: proto.String("Nice"),
			})
		} else {
			RunCommand("021FI_004.pv\n", mycli.worker)
			mycli.channel <- v.Info.Sender
			fmt.Println("param received")
			mycli.channel = make(chan types.JID)

		}
		// for range mycli.channel {
		// 	mycli.WAClient.SendMessage(context.Background(), v.Info.Sender, "", &waProto.Message{
		// 		Conversation: proto.String(<-mycli.channel),
		// 	})
		// }
	}

}

func main() {
	//subprocees init
	os.Setenv("HTTP_PROXY", "proxyIp:proxyPort")
	path, _ := filepath.Abs("./worker.exe")
	cmd := exec.Command(path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	//channel

	dbLog := waLog.Stdout("Database", "DEBUG", true)
	// Make sure you add appropriate DB connector imports, e.g. github.com/mattn/go-sqlite3 for SQLite
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	channel := make(chan types.JID)
	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	clients := MyClient{worker: stdin, WAClient: client, subprocess: cmd, channel: channel}
	clients.register()
	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	go func() {
		in := bufio.NewReader(stdout)
		for {
			sd, err := in.ReadString('\n')
			if err != nil {
				return
			}
			clients.WAClient.SendMessage(context.Background(), <-channel, "", &waProto.Message{
				Conversation: proto.String(sd),
			})
			fmt.Println(sd, "1s")
		}
	}()
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	if err := cmd.Wait(); err != nil {
		panic(err)
	}

	client.Disconnect()
}
