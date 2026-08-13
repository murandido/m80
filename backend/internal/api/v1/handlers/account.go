package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type AccountHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAccountHandler(db *gorm.DB) *AccountHandler {
	return &AccountHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

// archivedAt unwraps a gorm.DeletedAt for the wire. Shared by the account and the
// loyalty program responses, which both carry it so the panel can tell an archived
// record apart from a live one when it meets it embedded in a past issuance.
func archivedAt(deletedAt gorm.DeletedAt) *time.Time {
	if !deletedAt.Valid {
		return nil
	}
	return &deletedAt.Time
}

func toAccountResponse(account data.Account) schemas.AccountResponse {
	nicknames := make([]schemas.AccountNicknameResponse, 0, len(account.Nicknames))
	for _, nickname := range account.Nicknames {
		nicknames = append(nicknames, toAccountNicknameResponse(nickname))
	}

	var loyaltyProgram schemas.LoyaltyProgramResponse
	if account.LoyaltyProgram != nil {
		loyaltyProgram = schemas.LoyaltyProgramResponse{
			ID:               account.LoyaltyProgram.ID,
			Name:             account.LoyaltyProgram.Name,
			Code:             account.LoyaltyProgram.Code,
			Color:            account.LoyaltyProgram.Color,
			DefaultAirlineID: account.LoyaltyProgram.DefaultAirlineID,
			CreatedAt:        account.LoyaltyProgram.CreatedAt,
			UpdatedAt:        account.LoyaltyProgram.UpdatedAt,
			DeletedAt:        archivedAt(account.LoyaltyProgram.DeletedAt),
		}
	}

	return schemas.AccountResponse{
		ID:               account.ID,
		LoyaltyProgramID: account.LoyaltyProgramID,
		LoyaltyProgram:   loyaltyProgram,
		Login:            account.Login,
		Password:         account.Password,
		HolderID:         account.HolderID,
		AdministratorID:  account.AdministratorID,
		Nicknames:        nicknames,
		Complete:         account.Complete,
		CreatedAt:        account.CreatedAt,
		UpdatedAt:        account.UpdatedAt,
		DeletedAt:        archivedAt(account.DeletedAt),
	}
}

// CreateAccount handles the POST /accounts route
// @Summary      Criar conta
// @Description  Cria uma nova conta de programa de fidelidade no sistema.
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateAccountRequest true "Dados da nova conta"
// @Success      201  {object}  schemas.AccountSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Programa de fidelidade, titular ou administrador informado não existe"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar conta"
// @Router       /accounts [post]
func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateAccountRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	account := data.Account{
		LoyaltyProgramID: req.LoyaltyProgramID,
		Login:            req.Login,
		Password:         req.Password,
		HolderID:         req.HolderID,
		AdministratorID:  req.AdministratorID,
	}

	if req.Complete != nil {
		account.Complete = *req.Complete
	} else {
		account.Complete = account.ComputeComplete()
	}

	if err := h.DB.Create(&account).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Programa de fidelidade, titular ou administrador informado não existe")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create account: "+err.Error())
		return
	}

	// Reload: BeforeSave leaves Password encrypted in memory, and only a fresh
	// read triggers AfterFind to decrypt it back for the response.
	if err := h.DB.Preload("LoyaltyProgram").First(&account, "id = ?", account.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load created account: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Account created successfully", toAccountResponse(account))
}

// An account has no name of its own - the panel labels it by holder, then login, then
// alias (see account-card.tsx). "Alphabetical" therefore has to be an expression
// rather than a column, and it has to follow the same chain the operator reads: sort
// by holder and login alone and an account named by its alias lands in the NULL
// bucket at the end, which just looks like the list is not sorted.
//
// The alias comes from a sub-select for the same reason the suppliers view uses one:
// an account can hold several, and joining would duplicate the row. Oldest wins, so
// the position does not move when someone registers a second alias.
//
// Accounts with none of the three still sort last rather than jumping to the top on
// an empty string, and created_at breaks the ties so the order is stable.
const accountLabelOrder = `COALESCE(
	NULLIF(holders.name, ''),
	NULLIF(accounts.login, ''),
	(SELECT nickname FROM account_nicknames
		WHERE account_nicknames.account_id = accounts.id
		  AND account_nicknames.deleted_at IS NULL
		ORDER BY account_nicknames.created_at ASC
		LIMIT 1)
) COLLATE "pt-BR-x-icu" %s NULLS LAST, accounts.created_at ASC`

var accountOrder = map[string]string{
	"name_asc":     fmt.Sprintf(accountLabelOrder, "ASC"),
	"name_desc":    fmt.Sprintf(accountLabelOrder, "DESC"),
	"created_desc": "accounts.created_at DESC",
	"created_asc":  "accounts.created_at ASC",
}

// accountListQuery applies the optional search and the ordering. The join to holders is
// unconditional because the default ordering reads from it.
func accountListQuery(db *gorm.DB, params listQuery, programID string, complete *bool) *gorm.DB {
	query := db.Model(&data.Account{}).
		Joins("LEFT JOIN people AS holders ON holders.id = accounts.holder_id")

	if params.Q != "" {
		like := likePattern(params.Q)
		search := db.Where(unaccentedLike("accounts.login"), like).
			Or(unaccentedLike("holders.name"), like).
			Or("EXISTS (SELECT 1 FROM people AS administrators WHERE administrators.id = accounts.administrator_id AND "+
				unaccentedLike("administrators.name")+")", like).
			Or("EXISTS (SELECT 1 FROM loyalty_programs WHERE loyalty_programs.id = accounts.loyalty_program_id AND ("+
				unaccentedLike("loyalty_programs.name")+" OR "+unaccentedLike("loyalty_programs.code")+"))", like, like).
			Or(nicknameMatch("account_nicknames", "account_id", "accounts", true), like)
		query = query.Where(search)
	}

	if programID != "" {
		query = query.Where("accounts.loyalty_program_id = ?", programID)
	}
	if complete != nil {
		query = query.Where("accounts.complete = ?", *complete)
	}

	return query.Order(orderClause(params.Order, accountOrder, accountOrder["name_asc"]))
}

// GetAccounts handles the GET /accounts route.
// @Summary      Listar contas
// @Description  Retorna todas as contas cadastradas no sistema.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        q                   query  string  false  "Busca por login, titular, administrador, programa ou apelido"
// @Param        order               query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        complete            query  bool    false  "Somente contas completas (true) ou incompletas (false)"
// @Success      200  {object}  schemas.AccountListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar contas"
// @Router       /accounts [get]
func (h *AccountHandler) GetAccounts(w http.ResponseWriter, r *http.Request) {
	var accounts []data.Account

	query := accountListQuery(
		h.DB,
		parseListQuery(r),
		r.URL.Query().Get("loyalty_program_id"),
		parseOptionalBool(r.URL.Query().Get("complete")),
	)
	if err := query.Preload("Nicknames").Preload("LoyaltyProgram").Find(&accounts).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch accounts")
		return
	}

	resp := make([]schemas.AccountResponse, 0, len(accounts))
	for _, account := range accounts {
		resp = append(resp, toAccountResponse(account))
	}

	utils.SendJSON(w, r, http.StatusOK, "Accounts fetched successfully", resp)
}

// GetAccount handles the GET /accounts/{id} route
// @Summary      Buscar conta por ID
// @Description  Retorna os dados de uma conta específica identificada pelo UUID.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da conta"
// @Success      200  {object}  schemas.AccountSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Router       /accounts/{id} [get]
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var account data.Account
	if err := h.DB.Preload("Nicknames").Preload("LoyaltyProgram").First(&account, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Account not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Account fetched successfully", toAccountResponse(account))
}

// UpdateAccount handles the PUT /accounts/{id} route
// @Summary      Atualizar conta
// @Description  Atualiza os dados de uma conta existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                      true  "UUID da conta"
// @Param        payload body  schemas.UpdateAccountRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.AccountSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Programa de fidelidade, titular ou administrador informado não existe"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar conta"
// @Router       /accounts/{id} [put]
func (h *AccountHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var account data.Account
	if err := h.DB.Preload("Nicknames").First(&account, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Account not found")
		return
	}

	var req schemas.UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.LoyaltyProgramID != nil {
		account.LoyaltyProgramID = *req.LoyaltyProgramID
	}
	if req.Login != nil {
		account.Login = req.Login
	}
	if req.Password != nil {
		account.Password = req.Password
	}
	if req.HolderID != nil {
		account.HolderID = req.HolderID
	}
	if req.AdministratorID != nil {
		account.AdministratorID = req.AdministratorID
	}

	if req.Complete != nil {
		account.Complete = *req.Complete
	} else {
		account.Complete = account.ComputeComplete()
	}

	if err := h.DB.Save(&account).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Programa de fidelidade, titular ou administrador informado não existe")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update account: "+err.Error())
		return
	}

	// Reload: BeforeSave leaves Password encrypted in memory, and only a fresh
	// read triggers AfterFind to decrypt it back for the response.
	if err := h.DB.Preload("Nicknames").Preload("LoyaltyProgram").First(&account, "id = ?", account.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load updated account: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Account updated successfully", toAccountResponse(account))
}

// DeleteAccount handles the DELETE /accounts/{id} route.
// @Summary      Deletar conta
// @Description  Arquiva uma conta: ela some das listagens e dos seletores, mas as emissões antigas continuam mostrando em que conta foram feitas. Os apelidos da conta são arquivados junto, liberando o nome para outra conta do mesmo programa.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da conta"
// @Success      200  {object}  schemas.ErrorResponse "Conta arquivada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Conta ainda referenciada por outro registro"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao arquivar conta"
// @Router       /accounts/{id} [delete]
func (h *AccountHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// One transaction for the two, because an account whose aliases outlived it is
	// worse than one that was never archived: the alias is how the bot routes an
	// EMISSAO, so it would keep sending new issuances to an account nobody can see.
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", id).Delete(&data.AccountNickname{}).Error; err != nil {
			return err
		}
		return tx.Delete(&data.Account{}, "id = ?", id).Error
	})

	if err != nil {
		// Still reachable, just no longer through the issuance: people is referenced by
		// holder_id/administrator_id with RESTRICT and other rows may point here too.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir conta ainda referenciada por outro registro")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete account: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Account deleted successfully", nil)
}
