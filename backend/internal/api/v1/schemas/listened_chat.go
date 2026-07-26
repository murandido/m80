package schemas

import (
	"time"
)

// ListenedChatResponse defines how a listened_chats row is returned in the API
type ListenedChatResponse struct {
	ID               string    `json:"id"`
	ChatJID          string    `json:"chat_jid"`
	FriendlyName     *string   `json:"friendly_name"`
	Status           string    `json:"status"`
	Active           bool      `json:"active"`
	RequestedByID    string    `json:"requested_by_id"`
	RequestedByName  string    `json:"requested_by_name"`
	RequestedByPhone string    `json:"requested_by_phone"`
	ApprovedByID     *string   `json:"approved_by_id"`
	ApprovedByName   string    `json:"approved_by_name"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ListenedChatListResponse is the GET /bot/chats/pending route response
type ListenedChatListResponse struct {
	Success bool                   `json:"success" example:"true"`
	Message string                 `json:"message" example:"Chats pendentes listados com sucesso"`
	Data    []ListenedChatResponse `json:"data"`
}

// ListenedChatSingleResponse is the response from single-listened-chat routes
type ListenedChatSingleResponse struct {
	Success bool                 `json:"success" example:"true"`
	Message string               `json:"message" example:"Chat atualizado com sucesso"`
	Data    ListenedChatResponse `json:"data"`
}
