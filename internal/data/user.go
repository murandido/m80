package data

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID        string         `gorm:"primaryKey;type:uuid" json:"id"`
	Name      string         `gorm:"unique;not null" json:"name"`
	Phone     string         `gorm:"uniqueIndex;not null" json:"phone"`
	Password  string         `gorm:"not null" json:"-"`
	Role      string         `gorm:"default:'user'" json:"role"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	// if there isn't an ID, generates an UUID
	if u.ID == "" {
		u.ID = uuid.New().String()
	}

	// check the phone number
	if !utils.ValidateBRPhone(u.Phone) {
		return errors.New("Invalid phone number")
	}

	// hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}
