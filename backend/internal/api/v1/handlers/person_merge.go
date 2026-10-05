package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The same person shows up twice in the panel because the bot creates one whenever an
// EMISSAO names somebody it does not recognise (internal/bot/emissao_resolve.go). That is
// the right call - a message cannot be refused for want of a cadastro - but when nobody
// registers the alias afterwards, the duplicate stays, and it cannot be deleted: it is
// already the holder of an account, and accounts.holder_id is ON DELETE RESTRICT.
//
// Merging is what closes that: only three columns in the whole schema point at people
// (accounts.holder_id, accounts.administrator_id, person_nicknames.person_id), so
// repointing them empties the absorbed row and it can be deleted for real. The panel reaches
// this through the row merge (person_merge_row.go); this file holds the person half of it.

// personCarriedField is an optional column the merge can carry over from the absorbed
// person, with the label the preview shows. Name is absent on purpose: whoever survives
// keeps their own name, and the absorbed one becomes an alias instead.
type personCarriedField struct {
	label string
	of    func(*data.Person) **string
}

var personCarriedFields = []personCarriedField{
	{"CPF", func(p *data.Person) **string { return &p.CPF }},
	{"e-mail", func(p *data.Person) **string { return &p.Email }},
	{"telefone", func(p *data.Person) **string { return &p.Phone }},
	{"CEP", func(p *data.Person) **string { return &p.ZipCode }},
	{"estado", func(p *data.Person) **string { return &p.State }},
	{"cidade", func(p *data.Person) **string { return &p.City }},
	{"endereço", func(p *data.Person) **string { return &p.Address }},
	{"número", func(p *data.Person) **string { return &p.AddressNumber }},
	{"complemento", func(p *data.Person) **string { return &p.Complement }},
	{"bairro", func(p *data.Person) **string { return &p.Neighborhood }},
}

// carryPersonFields fills in, on keep, every optional column it is missing and drop has,
// and returns the labels of the ones it moved. The preview calls it on a copy to show the
// same list without writing anything.
func carryPersonFields(keep, drop *data.Person) []string {
	filled := make([]string, 0, len(personCarriedFields)+1)

	for _, field := range personCarriedFields {
		target, source := field.of(keep), field.of(drop)
		if *target == nil && *source != nil {
			*target = *source
			filled = append(filled, field.label)
		}
	}
	if keep.BirthDate == nil && drop.BirthDate != nil {
		keep.BirthDate = drop.BirthDate
		filled = append(filled, "data de nascimento")
	}

	return filled
}

// lockMergePair loads the two rows a merge is about to rewrite and holds them until the
// transaction ends. The bot writes to these same rows while resolving an EMISSAO, and
// ordering by id means two merges racing over the same pair queue up instead of deadlock.
func lockMergePair[T any](tx *gorm.DB, rows *[]T, keepID, dropID string) error {
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ?", []string{keepID, dropID}).
		Order("id").
		Find(rows).Error
}

// readMergeIDs pulls the surviving and absorbed ids off a request and rejects the two ways
// they can be nonsense before any query runs.
func readMergeIDs(w http.ResponseWriter, r *http.Request, sourceID string) (string, string, bool) {
	keepID := chi.URLParam(r, "id")

	if sourceID == "" {
		utils.SendError(w, r, http.StatusBadRequest, "Informe o cadastro que será absorvido")
		return "", "", false
	}
	if sourceID == keepID {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Selecione dois cadastros diferentes para mesclar")
		return "", "", false
	}

	return keepID, sourceID, true
}

// mergePeopleTx repoints everything that references drop onto keep, fills in what keep is
// missing and deletes drop. The caller holds the lock on both rows. It returns a warning when
// the absorbed name could not become an alias because somebody else already answers to it.
func mergePeopleTx(tx *gorm.DB, keep, drop data.Person, keepNickname bool) (string, error) {
	var warning string

	// Unscoped on both: leaving an archived account behind would keep it holding the
	// foreign key, and the DELETE at the end would be refused. UpdateColumn rather than
	// Update because this is not somebody editing the account.
	for _, column := range []string{"holder_id", "administrator_id"} {
		if err := tx.Unscoped().Model(&data.Account{}).
			Where(column+" = ?", drop.ID).
			UpdateColumn(column, keep.ID).Error; err != nil {
			return "", err
		}
	}

	// No collision to handle: person_nicknames.nickname is globally unique, so the two
	// can never be holding the same alias.
	if err := tx.Model(&data.PersonNickname{}).
		Where("person_id = ?", drop.ID).
		UpdateColumn("person_id", keep.ID).Error; err != nil {
		return "", err
	}

	if keepNickname {
		nickname, free, err := nicknameFromName(tx, keep, drop)
		if err != nil {
			return "", err
		}
		switch {
		case nickname == "":
			// Same name, or already an alias of the survivor - nothing to add.
		case free:
			if err := tx.Create(&data.PersonNickname{PersonID: keep.ID, Nickname: nickname}).Error; err != nil {
				return "", err
			}
		default:
			warning = fmt.Sprintf("O apelido %q já é de outro cadastro, então não foi criado.", nickname)
		}
	}

	// The delete comes before the copy on purpose: the CPF is unique across people -
	// idx_people_cpf_fingerprint since migration 38 - so writing the absorbed CPF onto the
	// survivor while that row still holds it is refused. The fingerprint follows the number
	// automatically: Person.BeforeSave derives it on the Save below.
	// Unscoped: a merge removes the absorbed row for real - archiving it would keep its CPF
	// holding the unique index and the person half-alive.
	if err := tx.Unscoped().Delete(&data.Person{}, "id = ?", drop.ID).Error; err != nil {
		return "", err
	}

	carryPersonFields(&keep, &drop)
	keep.Complete = keep.ComputeComplete()
	if err := tx.Save(&keep).Error; err != nil {
		return "", err
	}

	// Both people's aliases and both sets of accounts are one row now: one list of aliases,
	// and one administrator where the two rows did not disagree.
	if err := acct.AlignHolderAdministrator(tx, keep.ID); err != nil {
		return "", err
	}
	return warning, acct.UnifyHolderNicknames(tx, keep.ID)
}

// nicknameFromName reports the absorbed person's name as an alias for the survivor: the
// name itself, and whether it is free to be created. An empty name means there is nothing
// to add - the two are named the same, or the survivor already answers to it.
func nicknameFromName(db *gorm.DB, keep, drop data.Person) (string, bool, error) {
	if strings.EqualFold(strings.TrimSpace(keep.Name), strings.TrimSpace(drop.Name)) {
		return "", false, nil
	}

	var owner data.PersonNickname
	err := db.Where("LOWER(nickname) = LOWER(?)", drop.Name).First(&owner).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return drop.Name, true, nil
	case err != nil:
		return "", false, err
	case owner.PersonID == keep.ID:
		return "", false, nil
	default:
		return drop.Name, false, nil
	}
}

// sendPersonLookupError turns a missing row into a 404 and anything else into a 500.
func sendPersonLookupError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.SendError(w, r, http.StatusNotFound, "Pessoa não encontrada")
		return
	}
	utils.SendError(w, r, http.StatusInternalServerError, "Failed to merge people: "+err.Error())
}
