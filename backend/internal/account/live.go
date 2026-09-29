package account

import (
	"errors"
	"fmt"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Archiving an account or a person takes it out of the panel for good: old rows keep pointing
// at it, so history still reads right, but nothing new may. An issuance written against an
// archived account would be invisible from the accounts screen and count against a CPF limit
// nobody can see; an archived person made somebody's holder or administrator would bring a
// removed record back through the side door.

// ArchivedError is a write that would point at an archived record. What is the whole subject
// of the sentence, already agreeing in gender with "excluída"/"excluído" ("Esta conta foi
// excluída"), because the operator reads it as is.
type ArchivedError struct {
	What string
}

func (e *ArchivedError) Error() string {
	return fmt.Sprintf("%s e não pode receber nada novo", e.What)
}

// RequireLiveAccount refuses an account that is archived. A missing one is left to the
// foreign key, which already answers that.
func RequireLiveAccount(tx *gorm.DB, accountID string) error {
	var account data.Account
	err := tx.Unscoped().Select("id", "deleted_at").First(&account, "id = ?", accountID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if account.DeletedAt.Valid {
		return &ArchivedError{What: "Esta conta foi excluída"}
	}
	return nil
}

// RequireLivePerson refuses a person that is archived, for the same reason.
func RequireLivePerson(tx *gorm.DB, personID string) error {
	var person data.Person
	err := tx.Unscoped().Select("id", "deleted_at").First(&person, "id = ?", personID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if person.DeletedAt.Valid {
		return &ArchivedError{What: "Este titular foi excluído"}
	}
	return nil
}
