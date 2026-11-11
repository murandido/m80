package data

import "time"

// UserAccount grants a statement_client user read access to one loyalty account's
// statement. Staff never have rows here: their role is what gives them accounts.
type UserAccount struct {
	UserID    string    `gorm:"primaryKey;type:uuid" json:"user_id"`
	AccountID string    `gorm:"primaryKey;type:uuid" json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
}
