package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
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
// repointing them empties the absorbed row and the real DELETE that DeletePerson already
// does starts working on it.

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

// PreviewMergePerson handles the GET /people/{id}/merge-preview route.
// @Summary      Prévia da mesclagem de proprietários
// @Description  Conta o que a mesclagem vai mover do cadastro absorvido para o que fica. Não escreve nada.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        id         path   string  true  "UUID de quem fica"
// @Param        source_id  query  string  true  "UUID de quem será absorvido"
// @Success      200  {object}  schemas.MergePersonPreviewSingleResponse "Prévia calculada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não autorizado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Cadastros iguais"
// @Router       /people/{id}/merge-preview [get]
func (h *PeopleHandler) PreviewMergePerson(w http.ResponseWriter, r *http.Request) {
	keepID, dropID, ok := readMergeIDs(w, r, r.URL.Query().Get("source_id"))
	if !ok {
		return
	}

	var keep, drop data.Person
	if err := h.DB.First(&keep, "id = ?", keepID).Error; err != nil {
		sendPersonLookupError(w, r, err)
		return
	}
	if err := h.DB.First(&drop, "id = ?", dropID).Error; err != nil {
		sendPersonLookupError(w, r, err)
		return
	}

	// Unscoped: an archived account still holds holder_id, and those are exactly the rows
	// that make the absorbed person undeletable today. Counting only the living ones would
	// promise a merge smaller than the one that runs.
	var accounts, nicknames int64
	if err := h.DB.Unscoped().Model(&data.Account{}).
		Where("holder_id = ? OR administrator_id = ?", dropID, dropID).
		Count(&accounts).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count accounts: "+err.Error())
		return
	}
	if err := h.DB.Model(&data.PersonNickname{}).
		Where("person_id = ?", dropID).
		Count(&nicknames).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count nicknames: "+err.Error())
		return
	}

	preview := schemas.MergePersonPreviewResponse{
		Accounts:     accounts,
		Nicknames:    nicknames,
		FieldsToFill: carryPersonFields(&keep, &drop),
	}

	// Whether the absorbed name can become an alias is the one thing the panel cannot work
	// out on its own: the alias may already belong to a third person.
	if nickname, free, err := nicknameFromName(h.DB, keep, drop); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the nickname: "+err.Error())
		return
	} else if free {
		preview.NicknameFromName = nickname
	}

	utils.SendJSON(w, r, http.StatusOK, "Prévia calculada com sucesso", preview)
}

// MergePerson handles the POST /people/{id}/merge route.
// @Summary      Mesclar proprietários
// @Description  Move contas, apelidos e campos vazios do cadastro absorvido para o que fica, e então apaga o absorvido de verdade. Não tem desfazer.
// @Tags         Pessoas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                      true  "UUID de quem fica"
// @Param        request  body  schemas.MergePersonRequest  true  "Cadastro a absorver"
// @Success      200  {object}  schemas.MergePersonResultSingleResponse "Cadastros mesclados com sucesso"
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não autorizado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Cadastros iguais"
// @Router       /people/{id}/merge [post]
func (h *PeopleHandler) MergePerson(w http.ResponseWriter, r *http.Request) {
	var req schemas.MergePersonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	keepID, dropID, ok := readMergeIDs(w, r, req.SourceID)
	if !ok {
		return
	}

	// Default on: the duplicate exists because a name went unrecognised, so absorbing it
	// without teaching the survivor that name just means the next EMISSAO recreates it.
	keepNickname := req.KeepNickname == nil || *req.KeepNickname

	var warning string
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var pair []data.Person
		if err := lockMergePair(tx, &pair, keepID, dropID); err != nil {
			return err
		}
		if len(pair) != 2 {
			return gorm.ErrRecordNotFound
		}

		keep, drop := pair[0], pair[1]
		if keep.ID != keepID {
			keep, drop = drop, keep
		}

		// Unscoped on both: leaving an archived account behind would keep it holding the
		// foreign key, and the DELETE at the end would be refused. UpdateColumn rather than
		// Update because this is not somebody editing the account.
		for _, column := range []string{"holder_id", "administrator_id"} {
			if err := tx.Unscoped().Model(&data.Account{}).
				Where(column+" = ?", dropID).
				UpdateColumn(column, keepID).Error; err != nil {
				return err
			}
		}

		// No collision to handle: person_nicknames.nickname is globally unique, so the two
		// can never be holding the same alias.
		if err := tx.Model(&data.PersonNickname{}).
			Where("person_id = ?", dropID).
			UpdateColumn("person_id", keepID).Error; err != nil {
			return err
		}

		if keepNickname {
			nickname, free, err := nicknameFromName(tx, keep, drop)
			if err != nil {
				return err
			}
			switch {
			case nickname == "":
				// Same name, or already an alias of the survivor - nothing to add.
			case free:
				if err := tx.Create(&data.PersonNickname{PersonID: keep.ID, Nickname: nickname}).Error; err != nil {
					return err
				}
			default:
				warning = fmt.Sprintf("O apelido %q já é de outro cadastro, então não foi criado.", nickname)
			}
		}

		// The delete comes before the copy on purpose: idx_people_cpf is unique, so writing
		// the absorbed CPF onto the survivor while that row still holds it is refused.
		if err := tx.Delete(&data.Person{}, "id = ?", dropID).Error; err != nil {
			return err
		}

		carryPersonFields(&keep, &drop)
		keep.Complete = keep.ComputeComplete()
		return tx.Save(&keep).Error
	})

	if err != nil {
		sendPersonLookupError(w, r, err)
		return
	}

	var merged data.Person
	if err := h.DB.Preload("Nicknames").First(&merged, "id = ?", keepID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load the merged person: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Cadastros mesclados com sucesso", schemas.MergePersonResultResponse{
		Person:  toPersonResponse(merged),
		Warning: warning,
	})
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
