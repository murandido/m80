package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type ListenedChatHandler struct {
	DB *gorm.DB
}

func NewListenedChatHandler(db *gorm.DB) *ListenedChatHandler {
	return &ListenedChatHandler{DB: db}
}

func toListenedChatResponse(chat data.ListenedChat) schemas.ListenedChatResponse {
	resp := schemas.ListenedChatResponse{
		ID:            chat.ID,
		ChatJID:       chat.ChatJID,
		FriendlyName:  chat.FriendlyName,
		Status:        chat.Status,
		Active:        chat.Active,
		RequestedByID: chat.RequestedByID,
		ApprovedByID:  chat.ApprovedByID,
		CreatedAt:     chat.CreatedAt,
		UpdatedAt:     chat.UpdatedAt,
	}
	if chat.RequestedBy != nil {
		resp.RequestedByName = chat.RequestedBy.Name
		resp.RequestedByPhone = chat.RequestedBy.Phone
	}
	if chat.ApprovedBy != nil {
		resp.ApprovedByName = chat.ApprovedBy.Name
	}
	return resp
}

// GetPendingListenedChats handles the GET /bot/chats/pending route
// @Summary      Listar chats pendentes de aprovação
// @Description  Lista os chats do WhatsApp solicitados via !registrar-chat que ainda aguardam aprovação de um administrador.
// @Tags         Bot
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.ListenedChatListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao listar chats pendentes"
// @Router       /bot/chats/pending [get]
func (h *ListenedChatHandler) GetPendingListenedChats(w http.ResponseWriter, r *http.Request) {
	var chats []data.ListenedChat
	if err := h.DB.Preload("RequestedBy").Where("status = ?", "pending").Order("created_at asc").Find(&chats).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to list pending chats: "+err.Error())
		return
	}

	resp := make([]schemas.ListenedChatResponse, 0, len(chats))
	for _, chat := range chats {
		resp = append(resp, toListenedChatResponse(chat))
	}

	utils.SendJSON(w, r, http.StatusOK, "Chats pendentes listados com sucesso", resp)
}

// GetMonitoredListenedChats handles the GET /bot/chats/active route
// @Summary      Listar chats monitorados
// @Description  Lista os chats do WhatsApp atualmente monitorados (ativos) e os desativados permanentemente (que continuam visíveis, só sem ser monitorados). Chats parados via /stop (soft-stop) não entram aqui, mesmo comportamento de sempre.
// @Tags         Bot
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.ListenedChatListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao listar chats monitorados"
// @Router       /bot/chats/active [get]
func (h *ListenedChatHandler) GetMonitoredListenedChats(w http.ResponseWriter, r *http.Request) {
	var chats []data.ListenedChat
	if err := h.DB.Preload("RequestedBy").Preload("ApprovedBy").
		Where("active = ? OR status = ?", true, "deactivated").
		Order("created_at asc").Find(&chats).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to list monitored chats: "+err.Error())
		return
	}

	resp := make([]schemas.ListenedChatResponse, 0, len(chats))
	for _, chat := range chats {
		resp = append(resp, toListenedChatResponse(chat))
	}

	utils.SendJSON(w, r, http.StatusOK, "Chats monitorados listados com sucesso", resp)
}

// StopListenedChat handles the POST /bot/chats/{id}/stop route
// @Summary      Parar de monitorar um chat
// @Description  Desativa um chat atualmente monitorado (soft-stop: o registro e seu histórico de aprovação são mantidos, só active vira false). Apenas administradores podem acessar.
// @Tags         Bot
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "UUID do listened_chats"
// @Success      200  {object}  schemas.ListenedChatSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "Chat não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Chat não está ativo"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao parar de monitorar o chat"
// @Router       /bot/chats/{id}/stop [post]
func (h *ListenedChatHandler) StopListenedChat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var chat data.ListenedChat
	if err := h.DB.First(&chat, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Chat not found")
		return
	}

	if !chat.Active {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Chat is not currently active")
		return
	}

	chat.Active = false
	if err := h.DB.Save(&chat).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update chat: "+err.Error())
		return
	}

	if err := h.DB.Preload("RequestedBy").Preload("ApprovedBy").First(&chat, "id = ?", chat.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch updated chat")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Chat não será mais monitorado", toListenedChatResponse(chat))
}

// DeactivateListenedChat handles the POST /bot/chats/{id}/deactivate route
// @Summary      Desativar um chat monitorado permanentemente
// @Description  Diferente de /stop (soft-stop, invisível e reabre sozinho via !registrar-chat), desativar marca o chat com status "deactivated": ele continua visível em "chats monitorados" (cinza) e o bot nunca reabre sozinho - só via /reactivate. Apenas administradores podem acessar.
// @Tags         Bot
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "UUID do listened_chats"
// @Success      200  {object}  schemas.ListenedChatSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "Chat não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Chat não está ativo"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao desativar o chat"
// @Router       /bot/chats/{id}/deactivate [post]
func (h *ListenedChatHandler) DeactivateListenedChat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var chat data.ListenedChat
	if err := h.DB.First(&chat, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Chat not found")
		return
	}

	if chat.Status != "approved" || !chat.Active {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Chat is not currently active")
		return
	}

	chat.Status = "deactivated"
	chat.Active = false
	if err := h.DB.Save(&chat).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update chat: "+err.Error())
		return
	}

	if err := h.DB.Preload("RequestedBy").Preload("ApprovedBy").First(&chat, "id = ?", chat.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch updated chat")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Chat desativado com sucesso", toListenedChatResponse(chat))
}

// ReactivateListenedChat handles the POST /bot/chats/{id}/reactivate route
// @Summary      Reativar um chat desativado
// @Description  Reverte uma desativação (/deactivate), voltando o chat a ser monitorado. Não é uma nova decisão de aprovação, então não altera quem aprovou originalmente. Apenas administradores podem acessar.
// @Tags         Bot
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "UUID do listened_chats"
// @Success      200  {object}  schemas.ListenedChatSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "Chat não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Chat não está desativado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao reativar o chat"
// @Router       /bot/chats/{id}/reactivate [post]
func (h *ListenedChatHandler) ReactivateListenedChat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var chat data.ListenedChat
	if err := h.DB.First(&chat, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Chat not found")
		return
	}

	if chat.Status != "deactivated" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Chat is not currently deactivated")
		return
	}

	chat.Status = "approved"
	chat.Active = true
	if err := h.DB.Save(&chat).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update chat: "+err.Error())
		return
	}

	if err := h.DB.Preload("RequestedBy").Preload("ApprovedBy").First(&chat, "id = ?", chat.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch updated chat")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Chat reativado com sucesso", toListenedChatResponse(chat))
}

// ApproveListenedChat handles the POST /bot/chats/{id}/approve route
// @Summary      Aprovar chat pendente
// @Description  Aprova um chat solicitado via !registrar-chat, marcando-o como ativo. Apenas administradores podem acessar.
// @Tags         Bot
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "UUID do listened_chats"
// @Success      200  {object}  schemas.ListenedChatSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "Chat não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Chat já foi decidido"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao aprovar chat"
// @Router       /bot/chats/{id}/approve [post]
func (h *ListenedChatHandler) ApproveListenedChat(w http.ResponseWriter, r *http.Request) {
	h.decideListenedChat(w, r, "approved", true)
}

// DenyListenedChat handles the POST /bot/chats/{id}/deny route
// @Summary      Negar chat pendente
// @Description  Nega um chat solicitado via !registrar-chat. Apenas administradores podem acessar.
// @Tags         Bot
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "UUID do listened_chats"
// @Success      200  {object}  schemas.ListenedChatSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "Chat não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Chat já foi decidido"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao negar chat"
// @Router       /bot/chats/{id}/deny [post]
func (h *ListenedChatHandler) DenyListenedChat(w http.ResponseWriter, r *http.Request) {
	h.decideListenedChat(w, r, "denied", false)
}

// decideListenedChat is the shared approve/deny transition: both routes load
// the same way, guard against re-deciding an already-decided row the same
// way, and differ only in the resulting status/active values.
func (h *ListenedChatHandler) decideListenedChat(w http.ResponseWriter, r *http.Request, status string, active bool) {
	admin, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	id := chi.URLParam(r, "id")

	var chat data.ListenedChat
	if err := h.DB.First(&chat, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Chat not found")
		return
	}

	if chat.Status != "pending" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Chat has already been decided")
		return
	}

	chat.Status = status
	chat.Active = active
	adminID := admin.ID
	chat.ApprovedByID = &adminID

	if err := h.DB.Save(&chat).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update chat: "+err.Error())
		return
	}

	if err := h.DB.Preload("RequestedBy").Preload("ApprovedBy").First(&chat, "id = ?", chat.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch updated chat")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Chat atualizado com sucesso", toListenedChatResponse(chat))
}
