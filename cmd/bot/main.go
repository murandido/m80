package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/lib/pq"
	"github.com/murandido/m-80-backend/internal/bot"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func main() {
	// get the phone number to pair
	pairingPhoneNumber := os.Getenv("PAIRING_PHONE")

	// necessary logs
	dbLog := waLog.Stdout("Database", "INFO", true)
	clientLog := waLog.Stdout("Client", "INFO", true)

	ctx := context.Background()

	// get the db info
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)

	// connect to the db
	container, err := sqlstore.New(ctx, "postgres", dbURL, dbLog)
	if err != nil {
		panic(err)
	}

	// get the first device saved
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	client := whatsmeow.NewClient(deviceStore, clientLog)

	// instance the bot struct
	botInstance := bot.New(client, clientLog)

	client.AddEventHandler(botInstance.EventHandler)

	// if there's no device saved, try to connect
	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(ctx)

		err = client.Connect()
		if err != nil {
			panic(err)
		}

		<-qrChan

		code, err := client.PairPhone(ctx, pairingPhoneNumber, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
		if err != nil {
			fmt.Println("Error trying to pair phone:", err)
		} else {
			fmt.Println("Pairing code:", code)
		}
	} else {
		err = client.Connect()
		if err != nil {
			panic(err)
		}

		fmt.Println("✅ Bot reconnected with the saved session!")
	}

	// dont let the script finish
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c

	// disconnect the bot
	client.Disconnect()
	fmt.Println("Bot off.")
}
