package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	ntlm "github.com/bdwyertech/gontlm-proxy/cmd"
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
	callback       func(jid types.JID) types.JID
}

func (mycli *MyClient) register() {
	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)
}

func (mycli *MyClient) myEventHandler(evt interface{}) {
	// Handle event and access mycli.WAClient

	switch v := evt.(type) {
	case *events.Message:
		message := v.Message.GetConversation()
		fmt.Println("Received a message!", message)
		messagesID := []string{v.Info.ID}
		mycli.WAClient.MarkRead(messagesID, time.Now(), v.Info.MessageSource.Chat, v.Info.MessageSource.Sender)
		if message == "halo" {
			mycli.WAClient.SendMessage(context.Background(), v.Info.Sender, "", &waProto.Message{
				Conversation: proto.String("Nice"),
			})
		} else {
			RunCommand(message+"\n", mycli.worker)
			//mycli.channel <- v.Info.Sender
			fmt.Println("param received")
			mycli.callback(v.Info.Sender)
			//mycli.channel = make(chan types.JID)

		}
		// for range mycli.channel {
		// 	mycli.WAClient.SendMessage(context.Background(), v.Info.Sender, "", &waProto.Message{
		// 		Conversation: proto.String(<-mycli.channel),
		// 	})
		// }
	}

}

var senderID types.JID

func cb(jid types.JID) types.JID {
	senderID = jid
	return jid
}

type pv struct {
	TagName   string
	TimeStamp string
	Value     string
}

func main() {
	go func() {
		ntlm.Execute()
	}()
	// os.Setenv("HTTP_PROXY", "127.0.0.1:1111")
	//subprocees init
	//	os.Setenv("HTTP_PROXY", "proxyIp:proxyPort")
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
	clients := MyClient{worker: stdin, WAClient: client, subprocess: cmd, channel: channel, callback: cb}
	clients.register()
	if client.Store.ID == nil {
		time.Sleep(2 * time.Second)
		qrChan, _ := client.GetQRChannel(context.Background())
		client.SetProxyAddress("127.0.0.1:1111")
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
		time.Sleep(2 * time.Second)
		err = client.SetProxyAddress("http://127.0.0.1:1111")
		if err != nil {
			panic(err)
		}
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
			var payload string
			var pvs []pv
			json.Unmarshal([]byte(sd), &pvs)
			for i, s := range pvs {
				if i < len(pvs)-1 {
					payload += s.TagName + " : " + s.Value + "\n"
				} else {
					payload += s.TagName + " : " + s.Value
				}

			}
			clients.WAClient.SendMessage(context.Background(), senderID, "", &waProto.Message{
				Conversation: proto.String(payload),
			})
		}
	}()
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	if err := cmd.Wait(); err != nil {
		panic(err)
	}

	client.Disconnect()
}
