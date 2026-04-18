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
	Name      string         `gorm:"not null" json:"name"`
	Phone     string         `gorm:"uniqueIndex:idx_users_phone,where:deleted_at IS NULL;not null" json:"phone"`
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
		return errors.New("invalid phone number")
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

func (u *User) BeforeUpdate(tx *gorm.DB) (err error) {
	// check the phone number
	if !utils.ValidateBRPhone(u.Phone) {
		return errors.New("invalid phone number")
	}

	// prevent double hashing
	// bcrypt hashes are exactly 60 characters long
	// if the length is less than 60, it means it's a new plaintext password
	if u.Password != "" && len(u.Password) < 60 {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hashedPassword)
	}

	return nil
}
