package handlers

import (
	"net/http"

	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/utils"
)

// AzulConnectStatus handles GET /accounts/{id}/azul-statement/status
//
// The sync-state badge on its own - whether the account is connected, whether a connect login is
// in flight (and since when), plus the last balance and sync time. It is the cheap read the
// account card polls while a connect runs: the full entries endpoint resolves every link and
// loads the whole window, which is wasted work for a card that only wants to know "is it
// connecting yet, and did it finish". A never-synced account is a pending badge, not an error.
// @Summary      Estado da conexão/sincronização da conta Azul
// @Description  Devolve só o "crachá" da sincronização (conectado, conectando + desde quando, último saldo e última sincronização), sem carregar as linhas do extrato. É o que o card da conta consulta em intervalo enquanto um login roda em segundo plano, para saber quando ele terminou sem sobreviver a um F5 travado. Conta nunca sincronizada devolve estado "pending", não erro.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.AzulSyncStateSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao carregar o estado"
// @Router       /accounts/{id}/azul-statement/status [get]
func (h *StatementHandler) AzulConnectStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	state, err := azulsync.LoadState(h.DB, account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Estado carregado", h.toAzulSyncStateResponse(account, state))
}
