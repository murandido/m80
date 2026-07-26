package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// seedListenedChat creates a pending listened_chats row requested by a fresh
// user, with a unique chat_jid so seeds never collide within the same test.
func seedListenedChat(t *testing.T, tx *gorm.DB) (data.ListenedChat, data.User) {
	t.Helper()
	requester := createUser(t, tx, "Requester")

	chat := data.ListenedChat{
		ChatJID:       uuid.New().String() + "@g.us",
		RequestedByID: requester.ID,
		Status:        "pending",
		Active:        false,
	}
	create(t, tx, &chat)
	return chat, requester
}

func callDecision(tx *gorm.DB, handler *ListenedChatHandler, action, id, adminID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/bot/chats/"+id+"/"+action, nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.UserContextKey, data.User{ID: adminID, Role: "admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	if action == "approve" {
		handler.ApproveListenedChat(rec, req)
	} else {
		handler.DenyListenedChat(rec, req)
	}
	return rec
}

func callGetPending(handler *ListenedChatHandler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/bot/chats/pending", nil)
	rec := httptest.NewRecorder()
	handler.GetPendingListenedChats(rec, req)
	return rec
}

func callGetMonitored(handler *ListenedChatHandler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/bot/chats/active", nil)
	rec := httptest.NewRecorder()
	handler.GetMonitoredListenedChats(rec, req)
	return rec
}

func callStop(id string, handler *ListenedChatHandler) *httptest.ResponseRecorder {
	return callAction(id, "stop", handler.StopListenedChat)
}

func callDeactivate(id string, handler *ListenedChatHandler) *httptest.ResponseRecorder {
	return callAction(id, "deactivate", handler.DeactivateListenedChat)
}

func callReactivate(id string, handler *ListenedChatHandler) *httptest.ResponseRecorder {
	return callAction(id, "reactivate", handler.ReactivateListenedChat)
}

// callAction drives a POST /bot/chats/{id}/<action> handler directly,
// injecting the chi {id} param the same way the real router would. Stop,
// deactivate and reactivate all share this exact shape (id in the path, no
// body, no auth context needed since none of them read UserContextKey).
func callAction(id, action string, handlerFunc http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/bot/chats/"+id+"/"+action, nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	handlerFunc(rec, req)
	return rec
}

type listenedChatSingleResp struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message"`
	Data    schemas.ListenedChatResponse `json:"data"`
}

func TestApproveListenedChat_Success(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		rec := callDecision(tx, handler, "approve", chat.ID, admin.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp listenedChatSingleResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Data.Status != "approved" || !resp.Data.Active {
			t.Fatalf("data: got status=%q active=%v, want status=approved active=true", resp.Data.Status, resp.Data.Active)
		}
		if resp.Data.ApprovedByID == nil || *resp.Data.ApprovedByID != admin.ID {
			t.Fatalf("approved_by_id: got %v, want %q", resp.Data.ApprovedByID, admin.ID)
		}
		if resp.Data.ApprovedByName != admin.Name {
			t.Fatalf("approved_by_name: got %q, want %q", resp.Data.ApprovedByName, admin.Name)
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", chat.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Status != "approved" || !reloaded.Active {
			t.Fatalf("persisted chat: got status=%q active=%v", reloaded.Status, reloaded.Active)
		}
	})
}

func TestDenyListenedChat_Success(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		rec := callDecision(tx, handler, "deny", chat.ID, admin.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", chat.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Status != "denied" {
			t.Fatalf("status: got %q, want %q", reloaded.Status, "denied")
		}
		if reloaded.Active {
			t.Fatal("active: got true, want false after deny")
		}
	})
}

func TestDecideListenedChat_AlreadyDecided_Returns422(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		if rec := callDecision(tx, handler, "approve", chat.ID, admin.ID); rec.Code != http.StatusOK {
			t.Fatalf("first approve: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callDecision(tx, handler, "approve", chat.ID, admin.ID)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
	})
}

func TestDecideListenedChat_NotFound_Returns404(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		rec := callDecision(tx, handler, "approve", "00000000-0000-0000-0000-000000000000", admin.ID)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

func TestGetPendingListenedChats_OnlyReturnsPending(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		pending, requester := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		decided, _ := seedListenedChat(t, tx)
		if rec := callDecision(tx, handler, "deny", decided.ID, admin.ID); rec.Code != http.StatusOK {
			t.Fatalf("deny setup: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callGetPending(handler)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp struct {
			Data []schemas.ListenedChatResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		found := false
		for _, item := range resp.Data {
			if item.ID == decided.ID {
				t.Fatalf("denied chat %q should not appear in the pending list", decided.ID)
			}
			if item.ID == pending.ID {
				found = true
				if item.RequestedByName != requester.Name || item.RequestedByPhone != requester.Phone {
					t.Fatalf("requester info: got name=%q phone=%q, want name=%q phone=%q",
						item.RequestedByName, item.RequestedByPhone, requester.Name, requester.Phone)
				}
			}
		}
		if !found {
			t.Fatalf("expected pending chat %q in the list, got %+v", pending.ID, resp.Data)
		}
	})
}

func TestStopListenedChat_Success(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		if rec := callDecision(tx, handler, "approve", chat.ID, admin.ID); rec.Code != http.StatusOK {
			t.Fatalf("approve setup: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callStop(chat.ID, handler)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", chat.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Active {
			t.Fatal("active: got true, want false after stop")
		}
		// Soft-stop: the approval history is preserved, only active flips off.
		if reloaded.Status != "approved" {
			t.Fatalf("status: got %q, want %q (soft-stop keeps the decision on record)", reloaded.Status, "approved")
		}
	})
}

func TestStopListenedChat_NotActive_Returns422(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		handler := NewListenedChatHandler(tx)

		rec := callStop(chat.ID, handler)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
	})
}

func TestStopListenedChat_NotFound_Returns404(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		handler := NewListenedChatHandler(tx)

		rec := callStop("00000000-0000-0000-0000-000000000000", handler)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

func TestGetMonitoredListenedChats_IncludesActiveAndDeactivated_ExcludesStopped(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		active, _ := seedListenedChat(t, tx)
		deactivated, _ := seedListenedChat(t, tx)
		stopped, _ := seedListenedChat(t, tx)
		pending, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		for _, id := range []string{active.ID, deactivated.ID, stopped.ID} {
			if rec := callDecision(tx, handler, "approve", id, admin.ID); rec.Code != http.StatusOK {
				t.Fatalf("approve setup for %q: status %d, body %s", id, rec.Code, rec.Body.String())
			}
		}
		if rec := callDeactivate(deactivated.ID, handler); rec.Code != http.StatusOK {
			t.Fatalf("deactivate setup: status %d, body %s", rec.Code, rec.Body.String())
		}
		if rec := callStop(stopped.ID, handler); rec.Code != http.StatusOK {
			t.Fatalf("stop setup: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callGetMonitored(handler)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp struct {
			Data []schemas.ListenedChatResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		seen := map[string]schemas.ListenedChatResponse{}
		for _, item := range resp.Data {
			seen[item.ID] = item
			if item.ID == pending.ID {
				t.Fatalf("still-pending chat %q should not appear in the monitored list", pending.ID)
			}
			if item.ID == stopped.ID {
				t.Fatalf("soft-stopped chat %q should not appear in the monitored list (unchanged behavior)", stopped.ID)
			}
		}
		if item, ok := seen[active.ID]; !ok {
			t.Fatalf("expected active chat %q in the monitored list, got %+v", active.ID, resp.Data)
		} else if item.ApprovedByName != admin.Name {
			t.Fatalf("approved_by_name: got %q, want %q", item.ApprovedByName, admin.Name)
		}
		if item, ok := seen[deactivated.ID]; !ok {
			t.Fatalf("expected deactivated chat %q in the monitored list", deactivated.ID)
		} else if item.Status != "deactivated" || item.Active {
			t.Fatalf("deactivated chat: got status=%q active=%v", item.Status, item.Active)
		}
	})
}

func TestDeactivateListenedChat_Success(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		if rec := callDecision(tx, handler, "approve", chat.ID, admin.ID); rec.Code != http.StatusOK {
			t.Fatalf("approve setup: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callDeactivate(chat.ID, handler)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", chat.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Status != "deactivated" {
			t.Fatalf("status: got %q, want %q", reloaded.Status, "deactivated")
		}
		if reloaded.Active {
			t.Fatal("active: got true, want false after deactivate")
		}
	})
}

func TestDeactivateListenedChat_NotActive_Returns422(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		handler := NewListenedChatHandler(tx)

		rec := callDeactivate(chat.ID, handler)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
	})
}

func TestDeactivateListenedChat_AlreadyDeactivated_Returns422(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		if rec := callDecision(tx, handler, "approve", chat.ID, admin.ID); rec.Code != http.StatusOK {
			t.Fatalf("approve setup: status %d, body %s", rec.Code, rec.Body.String())
		}
		if rec := callDeactivate(chat.ID, handler); rec.Code != http.StatusOK {
			t.Fatalf("first deactivate: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callDeactivate(chat.ID, handler)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
	})
}

func TestDeactivateListenedChat_NotFound_Returns404(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		handler := NewListenedChatHandler(tx)

		rec := callDeactivate("00000000-0000-0000-0000-000000000000", handler)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

func TestReactivateListenedChat_Success(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		if rec := callDecision(tx, handler, "approve", chat.ID, admin.ID); rec.Code != http.StatusOK {
			t.Fatalf("approve setup: status %d, body %s", rec.Code, rec.Body.String())
		}
		if rec := callDeactivate(chat.ID, handler); rec.Code != http.StatusOK {
			t.Fatalf("deactivate setup: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callReactivate(chat.ID, handler)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", chat.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Status != "approved" || !reloaded.Active {
			t.Fatalf("persisted chat: got status=%q active=%v, want status=approved active=true", reloaded.Status, reloaded.Active)
		}
		if reloaded.ApprovedByID == nil || *reloaded.ApprovedByID != admin.ID {
			t.Fatalf("approved_by_id should be untouched by reactivate: got %v, want %q", reloaded.ApprovedByID, admin.ID)
		}
	})
}

func TestReactivateListenedChat_NotDeactivated_Returns422(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		chat, _ := seedListenedChat(t, tx)
		admin := createUser(t, tx, "Admin")
		handler := NewListenedChatHandler(tx)

		if rec := callDecision(tx, handler, "approve", chat.ID, admin.ID); rec.Code != http.StatusOK {
			t.Fatalf("approve setup: status %d, body %s", rec.Code, rec.Body.String())
		}

		rec := callReactivate(chat.ID, handler)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
	})
}

func TestReactivateListenedChat_NotFound_Returns404(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		handler := NewListenedChatHandler(tx)

		rec := callReactivate("00000000-0000-0000-0000-000000000000", handler)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}
