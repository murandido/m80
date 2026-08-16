package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Same story as person_merge.go, one level up: the bot opens an account whenever an
// EMISSAO names one it does not recognise, so a forgotten alias leaves the same loyalty
// account in the panel twice. Archiving the duplicate is all the panel can do today, and
// it does not help - the archived row keeps holding issuances.account_id, and its issuances
// stay split off from the account everybody actually uses.
//
// Merging moves the issuances and the aliases onto the surviving account, which empties
// the duplicate and lets it be removed for real rather than archived.

// auditActionUpdated is what a merge writes on every issuance it moves. There is no
// "merged" action and adding one would mean ALTER TYPE on the audit_action enum, which
// Postgres cannot take back in a down migration; "updated" is also honest - a column on
// the issuance changed - and the panel's history already renders the before/after shape.
const auditActionUpdated = "updated"

// PreviewMergeAccount handles the GET /accounts/{id}/merge-preview route.
// @Summary      Prévia da mesclagem de contas
// @Description  Conta o que a mesclagem vai mover da conta absorvida para a que fica. Não escreve nada.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id         path   string  true  "UUID da conta que fica"
// @Param        source_id  query  string  true  "UUID da conta que será absorvida"
// @Success      200  {object}  schemas.MergeAccountPreviewSingleResponse "Prévia calculada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não autorizado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Contas iguais ou de programas diferentes"
// @Router       /accounts/{id}/merge-preview [get]
func (h *AccountHandler) PreviewMergeAccount(w http.ResponseWriter, r *http.Request) {
	keepID, dropID, ok := readMergeIDs(w, r, r.URL.Query().Get("source_id"))
	if !ok {
		return
	}

	// Unscoped on both: absorbing a duplicate that somebody already archived is the most
	// likely case, since archiving is the only thing the panel offers today.
	var keep, drop data.Account
	if err := h.DB.Unscoped().First(&keep, "id = ?", keepID).Error; err != nil {
		sendAccountMergeError(w, r, err)
		return
	}
	if err := h.DB.Unscoped().First(&drop, "id = ?", dropID).Error; err != nil {
		sendAccountMergeError(w, r, err)
		return
	}
	if keep.LoyaltyProgramID != drop.LoyaltyProgramID {
		utils.SendError(w, r, http.StatusUnprocessableEntity, errDifferentPrograms)
		return
	}

	// Unscoped again: a soft-deleted issuance holds account_id exactly like a live one, so
	// it moves too - and a preview that ignored it would understate the merge.
	var issuances, nicknames int64
	if err := h.DB.Unscoped().Model(&data.Issuance{}).
		Where("account_id = ?", dropID).Count(&issuances).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count issuances: "+err.Error())
		return
	}
	if err := h.DB.Model(&data.AccountNickname{}).
		Where("account_id = ?", dropID).Count(&nicknames).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count nicknames: "+err.Error())
		return
	}

	colliding, err := collidingNicknames(h.DB, keepID, dropID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the nicknames: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Prévia calculada com sucesso", schemas.MergeAccountPreviewResponse{
		Issuances:          issuances,
		Nicknames:          nicknames,
		FieldsToFill:       accountFieldsToFill(keep, drop),
		CollidingNicknames: colliding,
	})
}

// MergeAccount handles the POST /accounts/{id}/merge route.
// @Summary      Mesclar contas
// @Description  Move emissões, apelidos e titular/administrador da conta absorvida para a que fica, e então apaga a absorvida de verdade. Não tem desfazer.
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                       true  "UUID da conta que fica"
// @Param        request  body  schemas.MergeAccountRequest  true  "Conta a absorver"
// @Success      200  {object}  schemas.AccountSingleResponse "Contas mescladas com sucesso"
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não autorizado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Contas iguais ou de programas diferentes"
// @Router       /accounts/{id}/merge [post]
func (h *AccountHandler) MergeAccount(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Não autenticado")
		return
	}

	var req schemas.MergeAccountRequest
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

	var differentPrograms bool
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var pair []data.Account
		if err := lockMergePair(tx.Unscoped(), &pair, keepID, dropID); err != nil {
			return err
		}
		if len(pair) != 2 {
			return gorm.ErrRecordNotFound
		}

		keep, drop := pair[0], pair[1]
		if keep.ID != keepID {
			keep, drop = drop, keep
		}
		if keep.LoyaltyProgramID != drop.LoyaltyProgramID {
			differentPrograms = true
			return errMergeRefused
		}

		if err := archiveCollidingNicknames(tx, keepID, dropID); err != nil {
			return err
		}

		// Unscoped: the aliases just archived above move too. They are what
		// issuances.account_nickname_id points at, so leaving them on a row that is about
		// to be deleted would take the record of which alias the EMISSAO came in by.
		if err := tx.Unscoped().Model(&data.AccountNickname{}).
			Where("account_id = ?", dropID).
			UpdateColumn("account_id", keepID).Error; err != nil {
			return err
		}

		if err := moveIssuances(tx, keepID, dropID, userCtx.ID); err != nil {
			return err
		}

		// UpdateColumns, not Save: Account.BeforeSave re-encrypts the password, and this
		// write has no business anywhere near it.
		updates := map[string]any{}
		if keep.HolderID == nil && drop.HolderID != nil {
			keep.HolderID = drop.HolderID
			updates["holder_id"] = *drop.HolderID
		}
		if keep.AdministratorID == nil && drop.AdministratorID != nil {
			keep.AdministratorID = drop.AdministratorID
			updates["administrator_id"] = *drop.AdministratorID
		}
		if len(updates) > 0 {
			updates["complete"] = keep.ComputeComplete()
			if err := tx.Model(&data.Account{}).Where("id = ?", keep.ID).
				UpdateColumns(updates).Error; err != nil {
				return err
			}
		}

		// Unscoped: without it GORM would archive the duplicate, which is the state the
		// operator is trying to get out of. Nothing points at the row by now.
		return tx.Unscoped().Delete(&data.Account{}, "id = ?", dropID).Error
	})

	if err != nil {
		if differentPrograms {
			utils.SendError(w, r, http.StatusUnprocessableEntity, errDifferentPrograms)
			return
		}
		sendAccountMergeError(w, r, err)
		return
	}

	var merged data.Account
	if err := h.DB.Preload("Nicknames").Preload("LoyaltyProgram").
		First(&merged, "id = ?", keepID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load the merged account: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Contas mescladas com sucesso", toAccountResponse(merged))
}

const errDifferentPrograms = "Só é possível mesclar contas do mesmo programa"

// errMergeRefused unwinds the transaction on a rule the caller already has a message for.
var errMergeRefused = errors.New("merge refused")

// collidingNicknames are the absorbed account's live aliases that the surviving one
// already answers to. Compared case-insensitively because that is how the bot matches
// them (internal/bot/emissao_resolve.go), which is stricter than the unique index and is
// what actually decides where an EMISSAO lands.
func collidingNicknames(db *gorm.DB, keepID, dropID string) ([]string, error) {
	var names []string
	err := db.Model(&data.AccountNickname{}).
		Where("account_id = ?", dropID).
		Where("LOWER(nickname) IN (?)", db.Model(&data.AccountNickname{}).
			Select("LOWER(nickname)").Where("account_id = ?", keepID)).
		Pluck("nickname", &names).Error
	if names == nil {
		names = []string{}
	}
	return names, err
}

// archiveCollidingNicknames retires the duplicated aliases before the rest are moved.
// Archiving rather than deleting: an issuance points at the alias row it came in by, and a
// delete would blank that (ON DELETE SET NULL). The uniqueness index only covers the
// living, so archiving is already enough to clear the way.
func archiveCollidingNicknames(tx *gorm.DB, keepID, dropID string) error {
	return tx.Model(&data.AccountNickname{}).
		Where("account_id = ?", dropID).
		Where("LOWER(nickname) IN (?)", tx.Model(&data.AccountNickname{}).
			Select("LOWER(nickname)").Where("account_id = ?", keepID)).
		UpdateColumn("deleted_at", time.Now()).Error
}

// moveIssuances repoints every issuance of the absorbed account, soft-deleted ones
// included - they hold account_id exactly like the live ones, which is the whole reason an
// account that ever issued cannot be deleted (see migration 29).
//
// Each one gets a history entry: the account on an issuance changed, and without a line
// saying why, whoever opens it later sees a field that moved on its own.
func moveIssuances(tx *gorm.DB, keepID, dropID, actorID string) error {
	var moved []string
	if err := tx.Unscoped().Model(&data.Issuance{}).
		Where("account_id = ?", dropID).
		Pluck("id", &moved).Error; err != nil {
		return err
	}
	if len(moved) == 0 {
		return nil
	}

	if err := tx.Unscoped().Model(&data.Issuance{}).
		Where("account_id = ?", dropID).
		UpdateColumn("account_id", keepID).Error; err != nil {
		return err
	}

	details, err := json.Marshal(map[string]any{
		"before": map[string]any{"account_id": dropID},
		"after":  map[string]any{"account_id": keepID},
		"reason": "account_merge",
	})
	if err != nil {
		return err
	}

	entries := make([]data.AuditLog, 0, len(moved))
	for _, issuanceID := range moved {
		entries = append(entries, data.AuditLog{
			IssuanceID:  issuanceID,
			ChangedByID: actorID,
			Action:      auditActionUpdated,
			Details:     datatypes.JSON(details),
		})
	}
	return tx.CreateInBatches(&entries, 100).Error
}

// accountFieldsToFill lists what the surviving account would inherit, for the preview.
func accountFieldsToFill(keep, drop data.Account) []string {
	filled := make([]string, 0, 2)
	if keep.HolderID == nil && drop.HolderID != nil {
		filled = append(filled, "titular")
	}
	if keep.AdministratorID == nil && drop.AdministratorID != nil {
		filled = append(filled, "administrador")
	}
	return filled
}

// sendAccountMergeError turns a missing row into a 404 and anything else into a 500.
func sendAccountMergeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
		return
	}
	utils.SendError(w, r, http.StatusInternalServerError, "Failed to merge accounts: "+err.Error())
}
