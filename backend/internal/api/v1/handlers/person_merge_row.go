package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// On the accounts screen a row is a holder and all of their loyalty accounts, so the
// duplicate the bot leaves behind is a whole row: a second "Fulana" with a second Smiles.
// Merging only the people would leave two Smiles accounts under one name, and the panel no
// longer has a per-account merge to finish the job - so the row merge does both, in one
// transaction, on top of the same two merges the older screens used (mergeAccountsTx and
// mergePeopleTx).
//
// The source can also be an account with no holder: those show up as rows of their own, and
// absorbing one into a holder is how an old orphan finally gets one.

var errAccountHasHolder = errors.New("account already has a holder")

// rowAccountPlan is what the merge does with each of the absorbed row's live accounts:
// fold it into the survivor's account in the same program, or just hand it to the survivor.
type rowAccountPlan struct {
	pairs [][2]data.Account // {keep, drop}
	moved []data.Account
}

// planRowAccounts pairs every absorbed account with the survivor's account in the same
// program, when there is one. The survivor's oldest account in a program wins, which is
// also the one the panel lists first in the row.
func planRowAccounts(db *gorm.DB, keepPersonID string, dropAccounts []data.Account) (rowAccountPlan, error) {
	var keepAccounts []data.Account
	if err := db.Preload("LoyaltyProgram").
		Where("holder_id = ?", keepPersonID).
		Order("created_at ASC").
		Find(&keepAccounts).Error; err != nil {
		return rowAccountPlan{}, err
	}

	byProgram := map[string]data.Account{}
	for _, account := range keepAccounts {
		if _, taken := byProgram[account.LoyaltyProgramID]; !taken {
			byProgram[account.LoyaltyProgramID] = account
		}
	}

	var plan rowAccountPlan
	for _, drop := range dropAccounts {
		if keep, ok := byProgram[drop.LoyaltyProgramID]; ok {
			plan.pairs = append(plan.pairs, [2]data.Account{keep, drop})
		} else {
			plan.moved = append(plan.moved, drop)
		}
	}
	return plan, nil
}

// readRowSource validates the request's source and returns which kind it is.
func readRowSource(w http.ResponseWriter, r *http.Request, personID, accountID string) (string, bool) {
	switch {
	case personID != "" && accountID != "":
		utils.SendError(w, r, http.StatusBadRequest, "Informe só um cadastro a absorver")
		return "", false
	case personID != "":
		_, _, ok := readMergeIDs(w, r, personID)
		return holderRowPerson, ok
	case accountID != "":
		return holderRowAccount, true
	default:
		utils.SendError(w, r, http.StatusBadRequest, "Informe o cadastro que será absorvido")
		return "", false
	}
}

// PreviewMergeRow handles the GET /people/{id}/merge-row-preview route.
// @Summary      Prévia da mesclagem de linhas da tela de contas
// @Description  Mostra o que absorver outra linha (pessoa ou conta sem titular) neste titular vai fazer: contas do mesmo programa que serão mescladas, contas que só mudam de titular e campos herdados. Não escreve nada.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        id                 path   string  true   "UUID do titular que fica"
// @Param        source_person_id   query  string  false  "UUID da pessoa a absorver"
// @Param        source_account_id  query  string  false  "UUID da conta sem titular a absorver"
// @Success      200  {object}  schemas.MergeRowPreviewSingleResponse "Prévia calculada com sucesso"
// @Failure      400  {object}  schemas.ErrorResponse "Origem ausente ou duplicada"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não autorizado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa ou conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Mesmo cadastro, ou conta que já tem titular"
// @Router       /people/{id}/merge-row-preview [get]
func (h *PeopleHandler) PreviewMergeRow(w http.ResponseWriter, r *http.Request) {
	keepID := chi.URLParam(r, "id")
	sourcePersonID := r.URL.Query().Get("source_person_id")
	sourceAccountID := r.URL.Query().Get("source_account_id")

	kind, ok := readRowSource(w, r, sourcePersonID, sourceAccountID)
	if !ok {
		return
	}

	var keep data.Person
	if err := h.DB.First(&keep, "id = ?", keepID).Error; err != nil {
		sendPersonLookupError(w, r, err)
		return
	}

	preview := schemas.MergeRowPreviewResponse{
		AccountsMerged: []schemas.MergeRowAccountPair{},
		FieldsToFill:   []string{},
	}

	var dropAccounts []data.Account
	if kind == holderRowPerson {
		var drop data.Person
		if err := h.DB.First(&drop, "id = ?", sourcePersonID).Error; err != nil {
			sendPersonLookupError(w, r, err)
			return
		}
		if err := h.DB.Where("holder_id = ?", drop.ID).Find(&dropAccounts).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to load accounts: "+err.Error())
			return
		}
		if err := h.DB.Unscoped().Model(&data.Account{}).
			Where("administrator_id = ?", drop.ID).Count(&preview.Administered).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to count accounts: "+err.Error())
			return
		}
		if err := h.DB.Model(&data.PersonNickname{}).
			Where("person_id = ?", drop.ID).Count(&preview.Nicknames).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to count nicknames: "+err.Error())
			return
		}
		preview.FieldsToFill = carryPersonFields(&keep, &drop)
		nickname, free, err := nicknameFromName(h.DB, keep, drop)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the nickname: "+err.Error())
			return
		}
		if free {
			preview.NicknameFromName = nickname
		}
	} else {
		// Unscoped: an archived account without a holder blocks the NOT NULL on holder_id
		// as much as a live one, and merging is how it gets one.
		var orphan data.Account
		if err := h.DB.Unscoped().First(&orphan, "id = ?", sourceAccountID).Error; err != nil {
			sendRowMergeError(w, r, err)
			return
		}
		if orphan.HolderID != nil {
			sendRowMergeError(w, r, errAccountHasHolder)
			return
		}
		dropAccounts = []data.Account{orphan}
	}

	plan, err := planRowAccounts(h.DB, keep.ID, dropAccounts)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to plan the merge: "+err.Error())
		return
	}
	preview.AccountsMoved = int64(len(plan.moved))

	for _, pair := range plan.pairs {
		keepAccount, dropAccount := pair[0], pair[1]
		entry := schemas.MergeRowAccountPair{
			KeepAccountID: keepAccount.ID,
			DropAccountID: dropAccount.ID,
			DropLogin:     dropAccount.Login,
		}
		if keepAccount.LoyaltyProgram != nil {
			entry.ProgramName = keepAccount.LoyaltyProgram.Name
		}
		if err := h.DB.Unscoped().Model(&data.Issuance{}).
			Where("account_id = ?", dropAccount.ID).Count(&entry.Issuances).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to count issuances: "+err.Error())
			return
		}
		if entry.CollidingNicknames, err = collidingNicknames(h.DB, keepAccount.ID, dropAccount.ID); err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the nicknames: "+err.Error())
			return
		}
		preview.AccountsMerged = append(preview.AccountsMerged, entry)
	}

	utils.SendJSON(w, r, http.StatusOK, "Prévia calculada com sucesso", preview)
}

// MergeRow handles the POST /people/{id}/merge-row route.
// @Summary      Mesclar linhas da tela de contas
// @Description  Absorve outra linha neste titular numa transação só: cada conta de programa que o titular já tem é mesclada na dele (emissões e apelidos migram), as demais só mudam de titular, e a pessoa absorvida é mesclada e apagada. Não tem desfazer.
// @Tags         Pessoas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                   true  "UUID do titular que fica"
// @Param        request  body  schemas.MergeRowRequest  true  "Linha a absorver"
// @Success      200  {object}  schemas.MergeRowResultSingleResponse "Contas mescladas com sucesso"
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não autorizado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa ou conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Mesmo cadastro, ou conta que já tem titular"
// @Router       /people/{id}/merge-row [post]
func (h *PeopleHandler) MergeRow(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Não autenticado")
		return
	}

	var req schemas.MergeRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	kind, ok := readRowSource(w, r, req.SourcePersonID, req.SourceAccountID)
	if !ok {
		return
	}
	keepID := chi.URLParam(r, "id")
	keepNickname := req.KeepNickname == nil || *req.KeepNickname

	var warning string
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var keep, dropPerson data.Person
		var dropAccounts []data.Account

		if kind == holderRowPerson {
			var pair []data.Person
			if err := lockMergePair(tx, &pair, keepID, req.SourcePersonID); err != nil {
				return err
			}
			if len(pair) != 2 {
				return gorm.ErrRecordNotFound
			}
			keep, dropPerson = pair[0], pair[1]
			if keep.ID != keepID {
				keep, dropPerson = dropPerson, keep
			}
			if err := tx.Where("holder_id = ?", dropPerson.ID).Find(&dropAccounts).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&keep, "id = ?", keepID).Error; err != nil {
				return err
			}
			var orphan data.Account
			if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&orphan, "id = ?", req.SourceAccountID).Error; err != nil {
				return err
			}
			if orphan.HolderID != nil {
				return errAccountHasHolder
			}
			dropAccounts = []data.Account{orphan}
		}

		plan, err := planRowAccounts(tx.Clauses(clause.Locking{Strength: "UPDATE"}), keep.ID, dropAccounts)
		if err != nil {
			return err
		}

		// The accounts first: once the people are merged, the survivor's and the absorbed
		// accounts sit under the same holder and can no longer be told apart.
		for _, pair := range plan.pairs {
			if err := mergeAccountsTx(tx, pair[0], pair[1], userCtx.ID); err != nil {
				return err
			}
		}

		if kind == holderRowPerson {
			// Hands every remaining account (the moved ones, archived ones, and the ones the
			// absorbed person administers) to the survivor, then deletes the absorbed person.
			mergeWarning, err := mergePeopleTx(tx, keep, dropPerson, keepNickname)
			warning = mergeWarning
			return err
		}

		for _, account := range plan.moved {
			account.HolderID = &keep.ID
			// UpdateColumns, not Save: Account.BeforeSave re-encrypts the password.
			// Unscoped: the orphan may be archived, and it stays archived - it only gains a holder.
			if err := tx.Unscoped().Model(&data.Account{}).Where("id = ?", account.ID).UpdateColumns(map[string]any{
				"holder_id": keep.ID,
				"complete":  account.ComputeComplete(),
			}).Error; err != nil {
				return err
			}
		}
		// The orphan's aliases join the row's list, and the row's reach the orphan; so does
		// the row's administrator.
		if err := acct.AlignHolderAdministrator(tx, keep.ID); err != nil {
			return err
		}
		return acct.UnifyHolderNicknames(tx, keep.ID)
	})

	if err != nil {
		sendRowMergeError(w, r, err)
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Contas mescladas com sucesso", schemas.MergeRowResultResponse{Warning: warning})
}

// sendRowMergeError maps the row merge's failures onto the statuses the panel expects.
func sendRowMergeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errAccountHasHolder):
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Essa conta já tem titular")
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.SendError(w, r, http.StatusNotFound, "Pessoa ou conta não encontrada")
	default:
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to merge: "+err.Error())
	}
}
