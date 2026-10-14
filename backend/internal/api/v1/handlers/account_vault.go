package handlers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// GetVaultAccounts handles the GET /accounts/vault route.
// @Summary      Listar contas do cofre
// @Description  Lista enxuta para o cofre da extensão: titular, programa, login e apelidos. Nunca traz senha.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   schemas.VaultAccountResponse
// @Failure      401  {object}  utils.StandardResponse
// @Router       /accounts/vault [get]
func (h *AccountHandler) GetVaultAccounts(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID           string
		Login        *string
		HolderName   *string
		ProgramID    string
		ProgramName  string
		ProgramCode  string
		ProgramColor string
		HasPassword  bool
	}

	// Joined by hand rather than preloaded: preloading the holder would decrypt their CPF and
	// phone for every account just to show a name, and the password must not leave the table
	// at all - only whether there is one.
	var rows []row
	err := h.DB.Model(&data.Account{}).
		Select(`accounts.id, accounts.login, holders.name AS holder_name,
		        loyalty_programs.id AS program_id, loyalty_programs.name AS program_name,
		        loyalty_programs.code AS program_code, loyalty_programs.color AS program_color,
		        (accounts.password IS NOT NULL AND accounts.password <> '') AS has_password`).
		Joins("JOIN loyalty_programs ON loyalty_programs.id = accounts.loyalty_program_id").
		Joins("LEFT JOIN people AS holders ON holders.id = accounts.holder_id").
		Order("holders.name ASC, loyalty_programs.name ASC").
		Scan(&rows).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the vault accounts")
		return
	}

	var nicknames []data.AccountNickname
	if err := h.DB.Select("account_id", "nickname").Find(&nicknames).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the account nicknames")
		return
	}
	byAccount := map[string][]string{}
	for _, nickname := range nicknames {
		byAccount[nickname.AccountID] = append(byAccount[nickname.AccountID], nickname.Nickname)
	}

	response := make([]schemas.VaultAccountResponse, 0, len(rows))
	for _, account := range rows {
		names := byAccount[account.ID]
		if names == nil {
			names = []string{}
		}
		response = append(response, schemas.VaultAccountResponse{
			ID:         account.ID,
			Login:      account.Login,
			HolderName: account.HolderName,
			Program: schemas.VaultProgram{
				ID: account.ProgramID, Name: account.ProgramName, Code: account.ProgramCode, Color: account.ProgramColor,
			},
			Nicknames:   names,
			HasPassword: account.HasPassword,
		})
	}

	utils.SendJSON(w, r, http.StatusOK, "Vault accounts fetched successfully", response)
}

// PostAccountCredentials handles the POST /accounts/{id}/credentials route.
// @Summary      Ler login e senha de uma conta (cofre)
// @Description  Devolve a senha descriptografada de uma conta e registra o acesso (quem, quando, para quê). Limitado por usuário.
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                             true  "UUID da conta"
// @Param        payload  body  schemas.AccountCredentialsRequest  true  "Para que a senha está sendo lida"
// @Success      200  {object}  schemas.AccountCredentialsResponse
// @Failure      400  {object}  utils.StandardResponse
// @Failure      404  {object}  utils.StandardResponse
// @Failure      422  {object}  utils.StandardResponse
// @Failure      429  {object}  utils.StandardResponse
// @Router       /accounts/{id}/credentials [post]
func (h *AccountHandler) PostAccountCredentials(w http.ResponseWriter, r *http.Request) {
	var req schemas.AccountCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// A malformed id is an account that does not exist, not a server error: Postgres would
	// reject it as a uuid and the caller would read a 500.
	accountID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(accountID); err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Account not found")
		return
	}

	var account data.Account
	if err := h.DB.First(&account, "id = ?", accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Account not found")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the account")
		return
	}
	if account.Password == nil || *account.Password == "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Esta conta não tem senha cadastrada")
		return
	}

	// The trail is written before the password leaves, and a failure to write it withholds the
	// password: a read nobody can account for is exactly what this endpoint exists to prevent.
	access := data.AccountCredentialAccess{
		AccountID: &account.ID,
		UserID:    user.ID,
		Action:    req.Action,
		ClientIP:  clientIP(r),
	}
	if err := h.DB.Create(&access).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to record the credential access")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Credentials fetched successfully", schemas.AccountCredentialsResponse{
		Login:    account.Login,
		Password: *account.Password,
	})
}

// clientIP is the address RealIP left in r.RemoteAddr, without the port. Behind the panel that
// is the browser address it forwarded; elsewhere it is whatever connected.
func clientIP(r *http.Request) *string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "" {
		return nil
	}
	return &host
}
