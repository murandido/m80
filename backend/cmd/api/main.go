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
func main() {
	data.Connect()

	// creates an default user if the db is empty
	var count int64
	data.DB.Model(&data.User{}).Count(&count)
	if count == 0 {
		fmt.Println("🌱 DB empty. Creating ADMIN user...")
		admin := data.User{
			Name:     "Admin",
			Phone:    "11999999999",
			Password: "12345678",
			Role:     "admin",
		}
		data.DB.Create(&admin)
		fmt.Println("👤 Created admin: 11999999999 / 12345678")
	}

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
