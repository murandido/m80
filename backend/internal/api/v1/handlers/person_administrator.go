package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// SetHolderAdministrator handles the PUT /people/{id}/administrator route.
// @Summary      Definir o administrador do titular
// @Description  Grava o administrador em todas as contas vivas do titular: um titular tem um administrador só, como tem uma lista só de apelidos. administrator_id vazio ou nulo tira o administrador de todas.
// @Tags         Pessoas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                                  true  "UUID do titular"
// @Param        request  body  schemas.SetHolderAdministratorRequest  true  "Administrador"
// @Success      200  {object}  schemas.ErrorResponse "Administrador definido"
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Titular ou administrador não encontrado"
// @Router       /people/{id}/administrator [put]
func (h *PeopleHandler) SetHolderAdministrator(w http.ResponseWriter, r *http.Request) {
	holderID := chi.URLParam(r, "id")

	var req schemas.SetHolderAdministratorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	var adminID *string
	if req.AdministratorID != nil && *req.AdministratorID != "" {
		adminID = req.AdministratorID
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&data.Person{}, "id = ?", holderID).Error; err != nil {
			return err
		}
		if adminID != nil {
			if err := tx.First(&data.Person{}, "id = ?", *adminID).Error; err != nil {
				return err
			}
		}
		return acct.SetHolderAdministrator(tx, holderID, adminID)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Titular ou administrador não encontrado")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to set the administrator: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Administrador definido", nil)
}
