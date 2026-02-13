package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/murandido/m-80-backend/internal/api"
	"github.com/murandido/m-80-backend/internal/data"
)

// @title           M-80 Backend API
// @version         0.1
// @description     API for controlling the backend of M-80 dashboard.
// @contact.name    murandido
// @contact.email   murandido@gmail.com
// @host            localhost:8080
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
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

	// config router
	r := api.NewRouter(data.DB)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🔥 Http server running on %s\n", port)
	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		panic(err)
	}

}
