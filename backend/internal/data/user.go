package data

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// The values of the user_role Postgres enum, created in migration 000001 and extended by
// 000032. They are plain strings because User.Role has to stay a string for gorm and for
// the JSON contract; a named type would ripple into both for no gain.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
	RoleDev   = "dev"
)

// UserRoles is the whitelist the API validates and filters against. Keep it in sync with
// the enum: a value that exists here but not in Postgres fails at INSERT time, not at
// validation time, which surfaces as a 500 instead of a 400.
var UserRoles = []string{RoleUser, RoleAdmin, RoleDev}

type User struct {
	ID        string         `gorm:"primaryKey;type:uuid" json:"id"`
	Name      string         `gorm:"not null" json:"name"`
	Phone     string         `gorm:"uniqueIndex:idx_users_phone,where:deleted_at IS NULL;not null" json:"phone"`
	Password  string         `gorm:"not null" json:"-"`
	Role      string         `gorm:"type:user_role;not null;default:'user'" json:"role"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	// if there isn't an ID, generates an UUID
	if u.ID == "" {
		u.ID = uuid.New().String()
	}

	// normalize before validating: WhatsApp's JID doesn't reliably carry
	// Brazil's optional mobile "nono dígito", so users.phone is kept in the
	// one canonical form both the bot and the login flow compare against.
	u.Phone = utils.NormalizeBRPhone(u.Phone)
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
	// normalize before validating - see BeforeCreate for why.
	u.Phone = utils.NormalizeBRPhone(u.Phone)
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
