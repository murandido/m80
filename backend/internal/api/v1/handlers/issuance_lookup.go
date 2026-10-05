package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"github.com/murandido/m-80-backend/internal/utils"
)

// LookupIssuanceBooking handles the POST /issuances/lookup route.
// @Summary      Prévia da reserva
// @Description  Consulta a reserva nos provedores da cia (a integração direta primeiro, a Traveos atrás) antes de a emissão existir, e devolve rota, data do voo e passageiro principal para o formulário. Não grava nada no banco; a Traveos pode importar a reserva, como faria no sync. O itinerário completo só é gravado pelo sync depois que a emissão é salva.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.IssuanceLookupRequest true "O que identifica a reserva na cia"
// @Success      200  {object}  schemas.IssuanceLookupResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Cia aérea não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "A cia não é integrada, falta um campo que ela exige, ou nenhum provedor encontrou a reserva"
// @Failure      502  {object}  schemas.ErrorResponse "O provedor recusou ou falhou"
// @Failure      503  {object}  schemas.ErrorResponse "Nenhum provedor configurado neste servidor"
// @Router       /issuances/lookup [post]
func (h *IssuanceHandler) LookupIssuanceBooking(w http.ResponseWriter, r *http.Request) {
	if h.Bookings == nil || h.Bookings.Empty() {
		// 503 and not 401, for the same reason as the sync: a 401 logs the operator out.
		utils.SendError(w, r, http.StatusServiceUnavailable,
			"A integração com as companhias aéreas não está configurada neste servidor")
		return
	}

	var req schemas.IssuanceLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// The same budget as the sync: a vendor import alone can take up to two minutes.
	ctx, cancel := context.WithTimeout(r.Context(), traveosSyncTimeout)
	defer cancel()

	preview, err := issuancesvc.Lookup(ctx, h.DB, h.Bookings, issuancesvc.LookupInput{
		AirlineID:           req.AirlineID,
		ProgramLocator:      req.ProgramLocator,
		AirlineLocator:      req.AirlineLocator,
		OriginIATACode:      req.OriginIATACode,
		MainPassengerName:   req.MainPassengerName,
		PassengerLastName:   req.PassengerLastName,
		LatamPurchaseNumber: req.LatamPurchaseNumber,
	})
	if err != nil {
		if errors.Is(err, issuancesvc.ErrLookupAirlineNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Airline not found")
			return
		}
		// Every other failure is one the sync can raise too, and the operator should
		// read the same message for it whether or not the issuance exists yet.
		h.respondToTraveosSyncError(w, r, "", err)
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Reserva encontrada", schemas.IssuanceLookupPreview{
		FlightDate:          preview.FlightDate,
		OriginIATACode:      preview.OriginIATACode,
		DestinationIATACode: preview.DestinationIATACode,
		MainPassengerName:   preview.MainPassengerName,
		LatamPurchaseNumber: preview.LatamPurchaseNumber,
		Source:              preview.Source,
	})
}
