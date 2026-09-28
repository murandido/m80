package handlers

import (
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/health"
	"github.com/murandido/m-80-backend/internal/utils"
)

// StatusHandler serves the panel's dependency lights. It is the only handler in
// this package with no *gorm.DB: it neither reads the database nor probes
// anything, it just projects whatever the background monitor last observed.
//
// That is what makes it safe to expose to everyone. The probes run on their own
// tickers in cmd/api, so a hundred people with the panel open cost a hundred
// map reads, not a hundred calls to a vendor that takes seconds to answer.
type StatusHandler struct {
	Monitor *health.Monitor
}

func NewStatusHandler(monitor *health.Monitor) *StatusHandler {
	return &StatusHandler{Monitor: monitor}
}

// GetSystemStatus handles the GET /status route.
// @Summary      Status das dependências
// @Description  Diz se o bot do WhatsApp, a Traveos e cada integração direta com companhia aérea estão no ar, a partir de sondas que rodam em background (bot a cada 60s, Traveos a cada 120s, cada companhia a cada 10min) somadas ao resultado das buscas reais. Aberta a qualquer usuário autenticado, de propósito: quem trabalha na grid de emissões é quem precisa saber que a vendor caiu. Não devolve o código de pareamento nem o JID do bot — esses continuam só no proxy dev `/bot/*`.
// @Tags         Status
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.SystemStatus
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Router       /status [get]
func (h *StatusHandler) GetSystemStatus(w http.ResponseWriter, r *http.Request) {
	snapshot := h.Monitor.Get()

	response := schemas.SystemStatus{
		Bot: schemas.DependencyStatus{
			State: string(snapshot.Bot.State),
			// The bot probe carries the whatsmeow session state as its detail;
			// no other dependency has a second dimension like this.
			Session:   snapshot.Bot.Detail,
			CheckedAt: snapshot.Bot.CheckedAt.UTC().Format(time.RFC3339),
		},
		Traveos: schemas.DependencyStatus{
			State:     string(snapshot.Traveos.State),
			CheckedAt: snapshot.Traveos.CheckedAt.UTC().Format(time.RFC3339),
		},
		Providers: make(map[string]schemas.DependencyStatus, len(snapshot.Providers)),
	}

	// No Detail is carried across, for the same reason the vendor's is not: an
	// airline's edge answers a block with HTML nobody wrote for us to read, and a
	// reason on screen would be invented. It goes to the log.
	for name, check := range snapshot.Providers {
		response.Providers[name] = schemas.DependencyStatus{
			State:     string(check.State),
			CheckedAt: check.CheckedAt.UTC().Format(time.RFC3339),
		}
	}

	utils.SendJSON(w, r, http.StatusOK, "System status", response)
}
