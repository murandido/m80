package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The HTTP half of promo_mode. Its sibling pdf-mode route has no coverage at all, which
// is the gap this deliberately does not copy: the pointer-vs-value trap below is
// invisible until the day a caller omits the field and a chat silently stops listening
// for promotions.

func callSetPromoMode(handler *ListenedChatHandler, id string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/bot/chats/"+id+"/promo-mode", bytes.NewReader(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	handler.SetListenedChatPromoMode(rec, req)
	return rec
}

func promoModeOf(t *testing.T, tx *gorm.DB, id string) bool {
	t.Helper()
	var chat data.ListenedChat
	if err := tx.Select("promo_mode").First(&chat, "id = ?", id).Error; err != nil {
		t.Fatalf("reading promo_mode back: %v", err)
	}
	return chat.PromoMode
}

func TestSetPromoModeTurnsTheModeOnAndOff(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		handler := &ListenedChatHandler{DB: tx}
		chat, _ := seedListenedChat(t, tx)

		rec := callSetPromoMode(handler, chat.ID, []byte(`{"enabled":true}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("turning on: status = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if !promoModeOf(t, tx, chat.ID) {
			t.Fatal("promo_mode should be on")
		}

		var payload schemas.ListenedChatSingleResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decoding response: %v", err)
		}
		// The response has to carry the new state, because the panel renders the switch
		// straight from it rather than re-fetching.
		if !payload.Data.PromoMode {
			t.Error("response should report promo_mode as on")
		}

		rec = callSetPromoMode(handler, chat.ID, []byte(`{"enabled":false}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("turning off: status = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if promoModeOf(t, tx, chat.ID) {
			t.Fatal("promo_mode should be off")
		}
	})
}

// The reason SetPromoModeRequest.Enabled is a pointer. Decoded into a plain bool, an
// omitted field reads as false, and a request that meant nothing would quietly turn the
// mode off for a chat nobody was asking about.
func TestSetPromoModeRejectsAMissingEnabledInsteadOfTurningItOff(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		handler := &ListenedChatHandler{DB: tx}
		chat, _ := seedListenedChat(t, tx)

		if rec := callSetPromoMode(handler, chat.ID, []byte(`{"enabled":true}`)); rec.Code != http.StatusOK {
			t.Fatalf("setup: status = %d (%s)", rec.Code, rec.Body.String())
		}

		rec := callSetPromoMode(handler, chat.ID, []byte(`{}`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
		}
		if !promoModeOf(t, tx, chat.ID) {
			t.Fatal("a request with no enabled field must leave the mode alone, not turn it off")
		}
	})
}

func TestSetPromoModeIsIndependentOfPDFMode(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		handler := &ListenedChatHandler{DB: tx}
		chat, _ := seedListenedChat(t, tx)

		if err := tx.Model(&data.ListenedChat{}).Where("id = ?", chat.ID).
			Update("pdf_mode", true).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}

		if rec := callSetPromoMode(handler, chat.ID, []byte(`{"enabled":true}`)); rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", chat.ID).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		if !reloaded.PDFMode {
			t.Error("writing promo_mode must not disturb pdf_mode")
		}
		if !reloaded.PromoMode {
			t.Error("promo_mode should be on")
		}
	})
}

func TestSetPromoModeAnswers404ForAnUnknownChat(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		handler := &ListenedChatHandler{DB: tx}

		rec := callSetPromoMode(handler, "00000000-0000-0000-0000-000000000000", []byte(`{"enabled":true}`))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
		}
	})
}
