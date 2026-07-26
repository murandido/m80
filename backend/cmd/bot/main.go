package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/murandido/m-80-backend/internal/bot"
	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func main() {
	// necessary logs
	dbLog := waLog.Stdout("Database", "INFO", true)
	clientLog := waLog.Stdout("Client", "INFO", true)

	ctx := context.Background()

	// WhatsApp rejects connections from an outdated client version ("Client
	// outdated (405)"), and the version baked into whatsmeow as a fallback
	// goes stale over time. Fetching the current one at boot keeps the bot
	// working without needing a dependency bump every time WhatsApp moves
	// the minimum version - failure here just falls back to that default.
	versionCtx, versionCancel := context.WithTimeout(ctx, 10*time.Second)
	latestVersion, err := whatsmeow.GetLatestVersion(versionCtx, nil)
	versionCancel()
	if err != nil {
		clientLog.Warnf("failed to fetch latest WhatsApp Web version, using built-in default: %v", err)
	} else {
		store.SetWAVersion(*latestVersion)
		clientLog.Infof("using WhatsApp Web version %s", latestVersion.String())
	}

	// get the db info
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)

	// connect to the shared app DB (internal/data models: users,
	// listened_chats, ...) - separate from whatsmeow's own sqlstore below,
	// which only holds session/device credentials. Reads the same DB_* env
	// vars and runs golang-migrate itself; safe to run concurrently with
	// cmd/api's own data.Connect() at boot, since golang-migrate takes a
	// Postgres advisory lock around the migration run.
	data.Connect()

	// connect to whatsmeow's own session store
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

	// botInstance is declared before newDeviceClient because the closure
	// needs to register botInstance.EventHandler on every client it builds,
	// including ones built long after this line runs - the closure captures
	// the variable, not its value, so this works once botInstance is
	// assigned below.
	var botInstance *bot.Bot

	// newDeviceClient lets the bot recover after any logout without a
	// process restart: whatsmeow deletes the device store (permanently
	// disabling the client that was using it) on every logout, remote or
	// through our own Logout call - see internal/bot.Bot.RecreateSession.
	newDeviceClient := func(ctx context.Context) (bot.ClientBase, bot.SessionClient, func() *types.JID, error) {
		device, err := container.GetFirstDevice(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		newClient := whatsmeow.NewClient(device, clientLog)
		newClient.AddEventHandler(botInstance.EventHandler)
		c, s, j := bot.WrapClient(newClient)
		return c, s, j, nil
	}

	// instance the bot struct - pairing is now admin-triggered on demand via
	// the internal HTTP API below, not auto-started here.
	botInstance = bot.New(client, clientLog, newDeviceClient, notifyStatusCallback(clientLog), data.DB, notifyChatRequestedCallback(clientLog))

	client.AddEventHandler(botInstance.EventHandler)

	// only auto-reconnect an already-paired session; otherwise stay
	// disconnected until POST /pair is called
	if client.Store.ID != nil {
		if err := client.Connect(); err != nil {
			panic(err)
		}
		fmt.Println("✅ Bot reconnected with the saved session!")
	}

	botPort := os.Getenv("BOT_PORT")
	if botPort == "" {
		botPort = "8081"
	}

	srv := &http.Server{
		Addr:              ":" + botPort,
		Handler:           newInternalRouter(botInstance),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		fmt.Printf("🔥 Bot internal API running on %s\n", botPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	// dont let the script finish
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	// disconnect the bot - through botInstance, not the local client var,
	// since RecreateSession may have swapped in a different one since boot
	botInstance.Disconnect()
	fmt.Println("Bot off.")
}

// notifyStatusCallback returns the StatusStore notify hook: a fire-and-forget
// POST to cmd/api's internal listener so it can broadcast the change over the
// existing SSE broker. Runs in its own goroutine so a slow/unreachable api
// never blocks the whatsmeow event handler.
func notifyStatusCallback(logger waLog.Logger) func() {
	callbackURL := os.Getenv("BOT_STATUS_CALLBACK_URL")
	return func() { postCallback(logger, callbackURL, "status change") }
}

// notifyChatRequestedCallback returns the hook internal/bot.Bot calls after
// !registrar-chat creates or reopens a listened_chats row - same bridge as
// notifyStatusCallback, just a different callback URL/endpoint on cmd/api's
// internal listener.
func notifyChatRequestedCallback(logger waLog.Logger) func() {
	callbackURL := os.Getenv("BOT_CHAT_REQUESTED_CALLBACK_URL")
	return func() { postCallback(logger, callbackURL, "chat request") }
}

// postCallback is the shared fire-and-forget POST behind both notify hooks
// above: empty body, own goroutine so a slow/unreachable api never blocks the
// whatsmeow event handler, no-op when the corresponding env var isn't set.
func postCallback(logger waLog.Logger, callbackURL, description string) {
	if callbackURL == "" {
		return
	}
	go func() {
		resp, err := http.Post(callbackURL, "application/json", bytes.NewReader(nil)) // #nosec G107 G704 -- callbackURL vem de env confiável, não de input de usuário
		if err != nil {
			logger.Warnf("failed to notify api of %s: %v", description, err)
			return
		}
		_ = resp.Body.Close()
	}()
}
