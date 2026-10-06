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
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/bookingproviders"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/health"
	"github.com/murandido/m-80-backend/internal/latam"
	"github.com/murandido/m-80-backend/internal/latamsync"
	"github.com/murandido/m-80-backend/internal/promotion"
	"github.com/murandido/m-80-backend/internal/smiles"
	"github.com/murandido/m-80-backend/internal/smilessync"
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

	// Who answers when an itinerary is needed. Traveos is the fallback rather than
	// the only option: each airline's own integration registers here as it is
	// built, and which one is actually tried for a carrier is decided per row by
	// airlines.direct_booking_enabled - see internal/issuance.resolveProviders.
	//
	// The provider is built inside the branch so that "no client" stays a real nil:
	// a nil *traveos.Client stored in an interface is not nil, and the registry
	// would then believe it has a fallback.
	var fallback booking.Provider
	if traveosClient != nil {
		fallback = traveos.NewProvider(traveosClient)
	}
	bookings := booking.NewRegistry(fallback)

	// The Azul client reads both the account statement and, now, the booking behind
	// an issuance. It is built once here rather than inside the sync block because
	// the two are independent: the booking lookup is anonymous and has nothing to do
	// with AZUL_SYNC_ENABLED, which only governs the statement ticker.
	//
	// A missing Firebase config disables the carrier with a warning instead of
	// stopping the panel from booting - the same tolerance Traveos gets, and the
	// same reason: an issuance is still worth registering without its itinerary.
	azulClient, err := azul.New()
	if err != nil {
		slog.Warn("azul integration is disabled", "err", err)
		azulClient = nil
	}
	bookingproviders.RegisterDirect(bookings, azulClient)

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

	// Every real lookup reports whether the carrier answered, which is a fresher and
	// far cheaper signal than any ticker for an airline that is actually being used.
	bookings.Watch(monitor.Observe)

	// And a slow synthetic probe underneath it, for the carriers nobody happened to
	// look anything up on today. It asks about a booking that cannot exist and reads
	// the refusal; ten minutes rather than two, because unlike the vendor these are
	// requests to an airline's public edge that this integration depends on not being
	// noticed by.
	for _, code := range bookings.Codes() {
		provider, _ := bookings.Direct(code)
		prober, ok := provider.(booking.Prober)
		if !ok {
			continue
		}
		go monitor.RunProvider(monitorCtx, provider.Name(), 10*time.Minute, health.BookingProviderProbe(prober))
	}

	// One Azul connector for the whole process, created here because three things start the same
	// login: the connect button (via the router), the ticker below when Azul stops recognising a
	// stored key, and saving a new password on such an account. Sharing it shares the tracker, so
	// two of them racing on one account never launch two browsers.
	var azulReader azulsync.Reader
	if azulClient != nil {
		// Only when built: a nil *azul.Client in the interface would read as a usable reader.
		azulReader = azulClient
	}
	azulConnector := azulsync.NewConnector(data.DB, azulsync.NewConnectTracker(), promotion.NewClient(), azulReader)

	// The Azul statement sync polls the connected inventory accounts on a ticker and stores
	// their statements for reconciliation. Off by default and never even constructed unless
	// AZUL_SYNC_ENABLED is set: the whole feature ships inert, to be turned on per environment
	// after a account is connected and watched. A missing Firebase config disables it with a
	// warning rather than stopping the panel from booting, exactly like Traveos above.
	if os.Getenv("AZUL_SYNC_ENABLED") == "true" {
		if azulClient == nil {
			slog.Warn("azul statement sync is disabled", "err", azul.ErrMissingConfig)
		} else {
			interval := azulSyncInterval()
			go azulsync.NewLoop(data.DB, azulClient, azulConnector, interval).Run(context.Background())
			slog.Info("azul statement sync enabled", "interval", interval.String())
		}
	}

	// One guard for the whole process, created here because two things need the same one: the
	// connect handlers (via the router) and the Smiles ticker below. The tenant revokes a
	// session whose refresh token is used twice, so a connect's first sync and a ticker round
	// must never overlap on one account - and two guards would not know about each other.
	smilesGuard := smilessync.NewSessionGuard()

	// The Smiles statement sync polls the connected inventory accounts on a ticker and stores
	// their statements for reconciliation. Off by default, exactly like the Azul one above, and
	// for the same reason: the feature ships inert and is turned on per environment once an
	// account has been connected and watched.
	//
	// Unlike Azul's, the client needs no configuration to build - what a round needs is the
	// account's own stored session - so there is nothing here that can disable it with a
	// warning.
	if os.Getenv("SMILES_SYNC_ENABLED") == "true" {
		interval := smilesSyncInterval()
		go smilessync.NewLoop(data.DB, smiles.New(), smilesGuard, interval).Run(context.Background())
		slog.Info("smiles statement sync enabled", "interval", interval.String())
	}

	// The LATAM balance read: renew each linked account's session and write its LATAM Pass
	// balance. No switch, unlike the two statement syncs above: with no account linked a pass is
	// one empty query, and linking one is itself the decision to read it. The client goes direct
	// rather than through LATAM_PROXY_URL - the account hosts answer the Oracle range, only the
	// booking ones refuse it (measured 2026-10-06). The guard is shared with the connect handler.
	latamGuard := latamsync.NewSessionGuard()
	latamAccounts := latam.NewAccountClient()
	go latamsync.NewLoop(data.DB, latamAccounts, latamGuard, latamsync.DefaultInterval).Run(context.Background())

	// config router
	r := api.NewRouter(data.DB, broker, bookings, monitor, smilesGuard, latamGuard, azulConnector)

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

// smilesSyncInterval reads SMILES_SYNC_INTERVAL as a Go duration (e.g. "10m"), falling back to
// the package default when it is unset or unparseable - a typo in the env should slow the sync
// to the safe default, not crash the boot.
func smilesSyncInterval() time.Duration {
	raw := os.Getenv("SMILES_SYNC_INTERVAL")
	if raw == "" {
		return smilessync.DefaultInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("SMILES_SYNC_INTERVAL is not a valid duration; using the default",
			"value", raw, "default", smilessync.DefaultInterval.String())
		return smilessync.DefaultInterval
	}
	return d
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
