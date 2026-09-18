package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/murandido/m-80-backend/internal/api"
	"github.com/murandido/m-80-backend/internal/azul"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/health"
	"github.com/murandido/m-80-backend/internal/traveos"
)

// @title           M-80 API
// @version         1.0
// @description     API de controle do painel M-80. Gerencie usuários e autenticação.
// @contact.name    murandido
// @contact.email   murandido@gmail.com
// @host            localhost:8080
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description     Informe o token JWT no formato: Bearer {token}
// seedInitialAdmin creates the first admin so an empty database is usable.
//
// Outside production the credentials are fixed and printed, and stay that way:
// that is the whole convenience, and everyone running the stack locally counts
// on it.
//
// In production the same convenience is a panel on a public domain whose admin
// password is written down in this repository, so the credentials have to come
// from the environment. When they are missing the API boots with no user at
// all - a panel somebody has to bootstrap by hand is a much smaller problem
// than one anybody who has read this repo can log into.
func seedInitialAdmin() {
	var count int64
	data.DB.Model(&data.User{}).Count(&count)
	if count > 0 {
		return
	}

	production := os.Getenv("ENV") == "production"
	phone, password := "11999999999", "12345678"
	if production {
		phone, password = os.Getenv("ADMIN_PHONE"), os.Getenv("ADMIN_PASSWORD")
		if phone == "" || password == "" {
			slog.Warn("no admin user was created: set ADMIN_PHONE and ADMIN_PASSWORD to seed the first account")
			return
		}
	}

	fmt.Println("🌱 DB empty. Creating ADMIN user...")
	admin := data.User{
		Name:     "Admin",
		Phone:    phone,
		Password: password,
		Role:     data.RoleAdmin,
	}
	// Refusing to boot beats booting without the only account that can log in.
	// The phone is validated in User.BeforeCreate, so a typo in ADMIN_PHONE
	// surfaces here rather than as an empty users table nobody looks at.
	if err := data.DB.Create(&admin).Error; err != nil {
		log.Fatal("❌ Failed to create the initial admin user: ", err)
	}

	if production {
		slog.Info("created the initial admin user", "phone", phone)
		return
	}
	fmt.Printf("👤 Created admin: %s / %s\n", phone, password)
}

func main() {
	data.Connect()

	seedInitialAdmin()

	if err := data.SeedAirports(); err != nil {
		log.Fatal("❌ Failed to seed airports: ", err)
	}

	// Airlines before loyalty programs: loyalty_programs.default_airline_id is NOT
	// NULL and is resolved from the airline code.
	if err := data.SeedAirlines(data.DB); err != nil {
		log.Fatal("❌ Failed to seed airlines: ", err)
	}
	if err := data.SeedLoyaltyPrograms(data.DB); err != nil {
		log.Fatal("❌ Failed to seed loyalty programs: ", err)
	}

	// No ordering constraint: the origins catalogue has no foreign keys of its own.
	if err := data.SeedPointBatchOrigins(); err != nil {
		log.Fatal("❌ Failed to seed point batch origins: ", err)
	}

	// Fills the Azul city-to-IATA table from the Go default the first time; never overwrites a
	// code the operator assigned by hand.
	if err := data.SeedAzulCityCodes(data.DB); err != nil {
		log.Fatal("❌ Failed to seed azul city codes: ", err)
	}

	// Encrypts whatever CPFs migration 38 left in plain text. Not fatal on purpose: it reports
	// the rows it could not rewrite and lets the API come up, because a single duplicate
	// document is not a reason to leave the panel down.
	if err := data.BackfillPersonCPFs(data.DB); err != nil {
		slog.Error("could not finish encrypting the CPFs in plain text", "err", err)
	}

	// One broker per process, threaded through the router the same way the DB is.
	// It lives as long as the server, so connected clients survive across requests.
	broker := events.NewBroker()

	// The Traveos client is optional at startup, unlike the database. Layer 3 is
	// opt-in per airline, so missing credentials must not stop the panel from
	// booting - the routes that need it answer 503 instead. A warning rather than
	// silence, because "the refetch button does nothing" is otherwise a long
	// afternoon.
	traveosClient, err := traveos.New()
	if err != nil {
		slog.Warn("traveos integration is disabled", "err", err)
		traveosClient = nil
	}

	// The dependency lights the panel shows. The probes run here, on their own
	// tickers, rather than inside the request that reads them: a vendor call
	// takes seconds, and one probe per browser refresh would be both slow and
	// rude to Traveos.
	//
	// Every observed *change* becomes an SSE event, which is what keeps the
	// browser from polling - see internal/api/CLAUDE.md. The monitor only calls
	// this on a transition, so a quiet day broadcasts nothing at all.
	monitor := health.NewMonitor(func() {
		broker.Broadcast(events.Event{Type: "system.status"})
	})
	monitorCtx := context.Background()
	// The bot is a call inside the Compose network and costs nothing; Traveos
	// is a real request to somebody else's production, so it goes slower.
	go monitor.Run(monitorCtx, health.DependencyBot, time.Minute, health.BotProbe(os.Getenv("BOT_INTERNAL_URL")))
	go monitor.Run(monitorCtx, health.DependencyTraveos, 2*time.Minute, health.TraveosProbe(traveosClient))

	// The Azul statement sync polls the connected inventory accounts on a ticker and stores
	// their statements for reconciliation. Off by default and never even constructed unless
	// AZUL_SYNC_ENABLED is set: the whole feature ships inert, to be turned on per environment
	// after a account is connected and watched. A missing Firebase config disables it with a
	// warning rather than stopping the panel from booting, exactly like Traveos above.
	if os.Getenv("AZUL_SYNC_ENABLED") == "true" {
		azulClient, err := azul.New()
		if err != nil {
			slog.Warn("azul statement sync is disabled", "err", err)
		} else {
			interval := azulSyncInterval()
			go azulsync.NewLoop(data.DB, azulClient, interval).Run(context.Background())
			slog.Info("azul statement sync enabled", "interval", interval.String())
		}
	}

	// config router
	r := api.NewRouter(data.DB, broker, traveosClient, monitor)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	internalPort := os.Getenv("API_INTERNAL_PORT")
	if internalPort == "" {
		internalPort = "8090"
	}

	internalSrv := &http.Server{
		Addr:              ":" + internalPort,
		Handler:           newInternalRouter(broker),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		fmt.Printf("🔥 Internal listener running on %s\n", internalPort)
		if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	fmt.Printf("🔥 Http server running on %s\n", port)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}

}

// azulSyncInterval reads AZUL_SYNC_INTERVAL as a Go duration (e.g. "10m"), falling back to the
// package default when it is unset or unparseable - a typo in the env should slow the sync to
// the safe default, not crash the boot.
func azulSyncInterval() time.Duration {
	raw := os.Getenv("AZUL_SYNC_INTERVAL")
	if raw == "" {
		return azulsync.DefaultInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("AZUL_SYNC_INTERVAL is not a valid duration; using the default",
			"value", raw, "default", azulsync.DefaultInterval.String())
		return azulsync.DefaultInterval
	}
	return d
}
