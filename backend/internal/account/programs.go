package account

import (
	"errors"
	"fmt"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// A holder has at most one live account per loyalty program. The panel shows a holder as one
// row with a badge per program, and the bot finds "Fulana's Azul" by her aliases - two live Azul
// accounts under one holder make both ambiguous, and the second one only ever appeared by
// accident (a program picked twice, or the bot not knowing she already had one).
//
// Archived accounts do not count: archiving is how an operator retires the old one.

// ProgramTakenError is a second live account for a holder in a program they already have.
type ProgramTakenError struct {
	Program string
}

func (e *ProgramTakenError) Error() string {
	return fmt.Sprintf("Este titular já tem uma conta na %s", e.Program)
}

// HolderAccountInProgram returns the holder's live account in the program, or nil when there
// is none. exceptID leaves one account out, so an account being edited does not collide with
// itself.
func HolderAccountInProgram(tx *gorm.DB, holderID, programID, exceptID string) (*data.Account, error) {
	query := tx.Preload("LoyaltyProgram").
		Where("holder_id = ? AND loyalty_program_id = ?", holderID, programID)
	if exceptID != "" {
		query = query.Where("id <> ?", exceptID)
	}

	var existing data.Account
	err := query.Order("created_at").First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// RequireFreeProgram refuses a holder/program pair that already has a live account.
func RequireFreeProgram(tx *gorm.DB, holderID, programID, exceptID string) error {
	existing, err := HolderAccountInProgram(tx, holderID, programID, exceptID)
	if err != nil || existing == nil {
		return err
	}
	return &ProgramTakenError{Program: programName(*existing)}
}
