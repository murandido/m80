package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/murandido/m-80-backend/internal/api"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
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

	// One broker per process, threaded through the router the same way the DB is.
	// It lives as long as the server, so connected clients survive across requests.
	broker := events.NewBroker()

	// config router
	r := api.NewRouter(data.DB, broker)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

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
