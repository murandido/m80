package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/carriers/onedrive"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/onedrivesync"
	"github.com/murandido/m-80-backend/internal/utils"
)

// MicrosoftConnectionHandler connects the Microsoft account the Tropas' workbooks are read
// with. Dev-only: whoever connects is consenting, for the whole installation, to the API
// reading every file shared with that account.
type MicrosoftConnectionHandler struct {
	Workbooks *onedrivesync.Service
	Validator *validator.Validate
}

func NewMicrosoftConnectionHandler(workbooks *onedrivesync.Service) *MicrosoftConnectionHandler {
	return &MicrosoftConnectionHandler{Workbooks: workbooks, Validator: validator.New()}
}

func (h *MicrosoftConnectionHandler) status(w http.ResponseWriter, r *http.Request, message string) {
	resp := schemas.MicrosoftConnectionResponse{Configured: h.Workbooks.Configured()}
	connection, err := h.Workbooks.Status()
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao ler a conexão Microsoft")
		return
	}
	if connection != nil {
		resp.Connected = true
		resp.State = &connection.State
		resp.Detail = connection.Detail
		resp.AccountName = connection.AccountName
		resp.AccountEmail = connection.AccountEmail
		resp.LastOKAt = connection.LastOKAt
		resp.ConnectedAt = &connection.CreatedAt
	}
	utils.SendJSON(w, r, http.StatusOK, message, resp)
}

// GetMicrosoftConnection handles the GET /microsoft-connection route
// @Summary      Conexão Microsoft
// @Description  Estado da conta Microsoft usada para ler as planilhas das Tropas no OneDrive. O token nunca é devolvido.
// @Tags         Tropas
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Router       /microsoft-connection [get]
func (h *MicrosoftConnectionHandler) GetMicrosoftConnection(w http.ResponseWriter, r *http.Request) {
	h.status(w, r, "Conexão Microsoft")
}

// StartMicrosoftConnection handles the POST /microsoft-connection/start route
// @Summary      Iniciar conexão Microsoft
// @Description  Devolve a URL de login da Microsoft (só leitura de arquivos). O retorno chega em /api/microsoft/callback do painel.
// @Tags         Tropas
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      503  {object}  schemas.ErrorResponse "Servidor sem o app Microsoft configurado"
// @Router       /microsoft-connection/start [post]
func (h *MicrosoftConnectionHandler) StartMicrosoftConnection(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}
	authorizeURL, err := h.Workbooks.Begin(user.ID)
	if errors.Is(err, onedrivesync.ErrNotConfigured) {
		utils.SendError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao iniciar o login Microsoft")
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "Abra a URL para entrar na Microsoft", map[string]string{"authorize_url": authorizeURL})
}

// FinishMicrosoftConnection handles the POST /microsoft-connection/callback route
// @Summary      Concluir conexão Microsoft
// @Description  Troca o código do retorno da Microsoft pelo token e guarda a conexão (substitui a anterior).
// @Tags         Tropas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.MicrosoftCallbackRequest  true  "Código e state do retorno"
// @Success      200  {object}  object
// @Failure      400  {object}  schemas.ErrorResponse "Dados inválidos"
// @Failure      422  {object}  schemas.ErrorResponse "Login expirado ou recusado pela Microsoft"
// @Router       /microsoft-connection/callback [post]
func (h *MicrosoftConnectionHandler) FinishMicrosoftConnection(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}
	var req schemas.MicrosoftCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || h.Validator.Struct(req) != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Retorno da Microsoft incompleto")
		return
	}
	_, err := h.Workbooks.Finish(r.Context(), user.ID, req.Code, req.State)
	if err != nil {
		// Every refusal, not only the unexpected ones: Microsoft's own reason (the AADSTS code
		// on a refused code exchange) is only in this line, and the panel just says "error".
		slog.Warn("microsoft connection: finish failed", "user_id", user.ID, "err", err)
	}
	switch {
	case errors.Is(err, onedrivesync.ErrNotConfigured):
		utils.SendError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	case errors.Is(err, onedrivesync.ErrUnknownState), errors.Is(err, onedrive.ErrNeedsLogin):
		utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil:
		utils.SendError(w, r, http.StatusBadGateway, "A Microsoft não respondeu como esperado. Tente de novo.")
		return
	}
	h.status(w, r, "Conta Microsoft conectada")
}

// DeleteMicrosoftConnection handles the DELETE /microsoft-connection route
// @Summary      Desconectar Microsoft
// @Description  Apaga o token guardado. As planilhas deixam de ser lidas até conectar de novo.
// @Tags         Tropas
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  object
// @Router       /microsoft-connection [delete]
func (h *MicrosoftConnectionHandler) DeleteMicrosoftConnection(w http.ResponseWriter, r *http.Request) {
	if err := h.Workbooks.Disconnect(); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao desconectar")
		return
	}
	h.status(w, r, "Conta Microsoft desconectada")
}
