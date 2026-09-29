package data

import "time"

// UserAgency grants a panel user access to one agency. dev users never have rows here:
// their role alone covers every agency (see access.LoadAgencyScope).
type UserAgency struct {
	UserID    string    `gorm:"primaryKey;type:uuid" json:"user_id"`
	AgencyID  string    `gorm:"primaryKey;type:uuid" json:"agency_id"`
	CreatedAt time.Time `json:"created_at"`
}
