package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The manual half of the CPF control. The automatic half - what the issuances say - is
// deliberately approximate: with no document to go on it matches passengers by name, and on
// the bot path that name is a surname. So the count is an instrument for a decision, not an
// audited fact, and the operator has to be able to correct it: merge two rows that were one
// person, attach a CPF the sync never brought, delete someone who was typed in wrong.
type AccountBeneficiaryHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAccountBeneficiaryHandler(db *gorm.DB) *AccountBeneficiaryHandler {
	return &AccountBeneficiaryHandler{DB: db, Validator: validator.New()}
}

func toBeneficiaryResponse(
	beneficiary data.AccountBeneficiary,
	uses int,
	lastUsedAt *time.Time,
) schemas.AccountBeneficiaryResponse {
	return schemas.AccountBeneficiaryResponse{
		ID:         beneficiary.ID,
		AccountID:  beneficiary.AccountID,
		Name:       beneficiary.Name,
		CPFMasked:  beneficiary.CPFMasked,
		HasCPF:     beneficiary.CPFFingerprint != nil,
		IsHolder:   beneficiary.IsHolder,
		Uses:       uses,
		LastUsedAt: lastUsedAt,
		CreatedAt:  beneficiary.CreatedAt,
	}
}

// GetAccountBeneficiaries handles the GET /accounts/{id}/beneficiaries route.
// @Summary      Listar beneficiários da conta
// @Description  Retorna as pessoas que já foram emitidas nesta conta, com quantas emissões cada uma tem e quando foi a última. O CPF só aparece mascarado — o número não é armazenado.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da conta"
// @Success      200  {object}  schemas.AccountBeneficiaryListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar beneficiários"
// @Router       /accounts/{id}/beneficiaries [get]
func (h *AccountBeneficiaryHandler) GetAccountBeneficiaries(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")

	var beneficiaries []data.AccountBeneficiary
	if err := h.DB.Where("account_id = ?", accountID).
		Order(textOrder("name", "ASC")).
		Find(&beneficiaries).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch beneficiaries")
		return
	}

	// One aggregate for the whole list rather than a preload of every use: the panel shows
	// a count and a date, and an account with years of history has thousands of use rows.
	type summary struct {
		BeneficiaryID string    `gorm:"column:beneficiary_id"`
		Uses          int       `gorm:"column:uses"`
		LastUsedAt    time.Time `gorm:"column:last_used_at"`
	}
	var summaries []summary
	err := h.DB.Table("account_beneficiary_uses AS uses").
		Select("uses.beneficiary_id, COUNT(*) AS uses, MAX(uses.used_at) AS last_used_at").
		Joins("JOIN account_beneficiaries AS beneficiaries ON beneficiaries.id = uses.beneficiary_id").
		Where("beneficiaries.account_id = ?", accountID).
		Group("uses.beneficiary_id").
		Scan(&summaries).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to summarise the beneficiaries")
		return
	}

	byID := make(map[string]summary, len(summaries))
	for _, row := range summaries {
		byID[row.BeneficiaryID] = row
	}

	resp := make([]schemas.AccountBeneficiaryResponse, 0, len(beneficiaries))
	for _, beneficiary := range beneficiaries {
		row, ok := byID[beneficiary.ID]
		if !ok {
			// A beneficiary an operator added by hand and has not issued for yet.
			resp = append(resp, toBeneficiaryResponse(beneficiary, 0, nil))
			continue
		}
		lastUsedAt := row.LastUsedAt
		resp = append(resp, toBeneficiaryResponse(beneficiary, row.Uses, &lastUsedAt))
	}

	utils.SendJSON(w, r, http.StatusOK, "Beneficiaries fetched successfully", resp)
}

// CreateAccountBeneficiary handles the POST /accounts/{id}/beneficiaries route.
// @Summary      Adicionar beneficiário à conta
// @Description  Registra à mão uma pessoa emitida nesta conta — para o caso em que a emissão não está no sistema. Se o CPF for informado, apenas a impressão digital e a máscara são guardadas; o número não.
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                                     true  "UUID da conta"
// @Param        payload body  schemas.CreateAccountBeneficiaryRequest    true  "Dados do beneficiário"
// @Success      201  {object}  schemas.AccountBeneficiarySingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Beneficiário já existe nesta conta"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar beneficiário"
// @Router       /accounts/{id}/beneficiaries [post]
func (h *AccountBeneficiaryHandler) CreateAccountBeneficiary(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")

	var req schemas.CreateAccountBeneficiaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// A manual entry counts against the cap the same way an issuance does - that is the
	// point of typing it in. Without an issuance to date it, "now" is the best answer, and
	// UsedAt lets the operator say otherwise when they are recording something older.
	usedAt := time.Now()
	if req.UsedAt != nil {
		usedAt = *req.UsedAt
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		return acct.RegisterManual(tx, accountID, req.Name, req.CPF, usedAt)
	})
	if err != nil {
		if errors.Is(err, acct.ErrInvalidCPF) {
			utils.SendError(w, r, http.StatusBadRequest, "CPF inválido")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create beneficiary: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Beneficiary created successfully", nil)
}

// UpdateAccountBeneficiary handles the PATCH /accounts/{id}/beneficiaries/{beneficiaryID} route.
// @Summary      Corrigir um beneficiário
// @Description  Corrige o nome ou informa o CPF de um beneficiário. O CPF é guardado como impressão digital e máscara — o número não fica no banco.
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id             path  string                                  true  "UUID da conta"
// @Param        beneficiaryID  path  string                                  true  "UUID do beneficiário"
// @Param        payload        body  schemas.UpdateAccountBeneficiaryRequest true  "Campos a corrigir"
// @Success      200  {object}  schemas.AccountBeneficiarySingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Beneficiário não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar beneficiário"
// @Router       /accounts/{id}/beneficiaries/{beneficiaryID} [patch]
func (h *AccountBeneficiaryHandler) UpdateAccountBeneficiary(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	beneficiaryID := chi.URLParam(r, "beneficiaryID")

	var req schemas.UpdateAccountBeneficiaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	var beneficiary data.AccountBeneficiary
	if err := h.DB.First(&beneficiary, "id = ? AND account_id = ?", beneficiaryID, accountID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Beneficiary not found")
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		return acct.Correct(tx, &beneficiary, req.Name, req.CPF, req.IsHolder)
	})
	if err != nil {
		if errors.Is(err, acct.ErrInvalidCPF) {
			utils.SendError(w, r, http.StatusBadRequest, "CPF inválido")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update beneficiary: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Beneficiary updated successfully",
		toBeneficiaryResponse(beneficiary, 0, nil))
}

// DeleteAccountBeneficiary handles the DELETE /accounts/{id}/beneficiaries/{beneficiaryID} route.
// @Summary      Remover um beneficiário
// @Description  Apaga um beneficiário e as emissões que ele consumiu nesta conta. Use para quem entrou errado — não para desfazer um cancelamento, já que a vaga usada não volta.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id             path  string  true  "UUID da conta"
// @Param        beneficiaryID  path  string  true  "UUID do beneficiário"
// @Success      200  {object}  schemas.ErrorResponse "Beneficiário removido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Beneficiário não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao remover beneficiário"
// @Router       /accounts/{id}/beneficiaries/{beneficiaryID} [delete]
func (h *AccountBeneficiaryHandler) DeleteAccountBeneficiary(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	beneficiaryID := chi.URLParam(r, "beneficiaryID")

	var beneficiary data.AccountBeneficiary
	if err := h.DB.First(&beneficiary, "id = ? AND account_id = ?", beneficiaryID, accountID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Beneficiary not found")
		return
	}

	// The uses go with it: the FK cascades. A hard delete rather than a soft one because
	// this is a correction of something that never happened, not a record of it ending.
	if err := h.DB.Delete(&beneficiary).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete beneficiary")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Beneficiary deleted successfully", nil)
}

// MergeAccountBeneficiaries handles the POST /accounts/{id}/beneficiaries/{beneficiaryID}/merge route.
// @Summary      Juntar dois beneficiários
// @Description  Move as emissões de um beneficiário para outro e apaga o de origem. É a correção do caso em que a mesma pessoa entrou duas vezes com grafias diferentes.
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id             path  string                                 true  "UUID da conta"
// @Param        beneficiaryID  path  string                                 true  "UUID do beneficiário que permanece"
// @Param        payload        body  schemas.MergeAccountBeneficiaryRequest true  "Beneficiário a ser absorvido"
// @Success      200  {object}  schemas.ErrorResponse "Beneficiários juntados"
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Beneficiário não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao juntar beneficiários"
// @Router       /accounts/{id}/beneficiaries/{beneficiaryID}/merge [post]
func (h *AccountBeneficiaryHandler) MergeAccountBeneficiaries(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	survivorID := chi.URLParam(r, "beneficiaryID")

	var req schemas.MergeAccountBeneficiaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}
	if req.SourceID == survivorID {
		utils.SendError(w, r, http.StatusBadRequest, "Um beneficiário não pode ser juntado a si mesmo")
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		return acct.Merge(tx, accountID, survivorID, req.SourceID)
	})
	if err != nil {
		if errors.Is(err, acct.ErrBeneficiaryNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Beneficiary not found")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to merge beneficiaries: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Beneficiaries merged successfully", nil)
}

// ReconcileAccountBeneficiaries handles the POST /accounts/{id}/beneficiaries/reconcile route.
// @Summary      Reconstruir os beneficiários a partir das emissões
// @Description  Descarta o que foi ajustado à mão e recalcula a lista pelas emissões da conta. Admin-only, e sem desfazer.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da conta"
// @Success      200  {object}  schemas.ErrorResponse "Beneficiários reconstruídos"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Sem permissão"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao reconstruir beneficiários"
// @Router       /accounts/{id}/beneficiaries/reconcile [post]
func (h *AccountBeneficiaryHandler) ReconcileAccountBeneficiaries(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		return acct.Reconcile(tx, accountID)
	})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to reconcile beneficiaries: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Beneficiaries reconciled successfully", nil)
}
