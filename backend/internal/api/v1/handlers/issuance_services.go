package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The seats and the baggage of an issuance, readable and writable from the
// panel. Traveos lets an operator fix both after the fact and we could not, so a
// seat the airline reassigned at the gate had no way into the ticket the client
// receives.
//
// These talk to GORM directly rather than going through internal/issuance. That
// package exists because the bot needs the same rules the API does; the bot has
// no reason to correct a seat, so putting this there would be building a shared
// home for one occupant.
//
// No audit_logs entry, the same decision already recorded for the grid's
// autosave in internal/api/CLAUDE.md: this is correction of a detail, and a row
// of history per seat would bury the entries that matter.

var errUnknownReference = errors.New("the payload names a passenger or a flight that is not on this issuance")

// GetIssuanceSeats handles the GET /issuances/{id}/seats route.
// @Summary      Assentos da emissão
// @Description  Devolve um assento por passageiro por trecho, na mesma forma que o PUT aceita de volta.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {object}  schemas.IssuanceSeatsResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar assentos"
// @Router       /issuances/{id}/seats [get]
func (h *IssuanceHandler) GetIssuanceSeats(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")
	if !h.issuanceExistsForServices(w, r, issuanceID) {
		return
	}

	var rows []data.IssuancePassengerFlight
	if err := h.DB.
		Where("passenger_id IN (?)", h.passengerIDsOf(issuanceID)).
		Find(&rows).Error; err != nil {
		slog.Error("failed to read the seats of an issuance", "issuance_id", issuanceID, "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch seats")
		return
	}

	seats := make([]schemas.IssuanceSeat, 0, len(rows))
	for _, row := range rows {
		seats = append(seats, schemas.IssuanceSeat{
			PassengerID: row.PassengerID,
			FlightID:    row.FlightID,
			Seat:        row.Seat,
		})
	}
	utils.SendJSON(w, r, http.StatusOK, "Seats fetched successfully", seats)
}

// PutIssuanceSeats handles the PUT /issuances/{id}/seats route.
// @Summary      Substituir os assentos da emissão
// @Description  Substitui o conjunto inteiro de assentos numa transação. O corpo é o estado completo, nunca um delta.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                            true  "UUID da emissão"
// @Param        payload  body  schemas.PutIssuanceSeatsRequest    true  "Assentos"
// @Success      200  {object}  schemas.IssuanceSeatsResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Passageiro ou trecho não pertence a esta emissão"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao gravar assentos"
// @Router       /issuances/{id}/seats [put]
func (h *IssuanceHandler) PutIssuanceSeats(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")
	if !h.issuanceExistsForServices(w, r, issuanceID) {
		return
	}

	var req schemas.PutIssuanceSeatsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		passengers, flights, err := ownedIDs(tx, issuanceID)
		if err != nil {
			return err
		}

		for _, seat := range req.Seats {
			if !passengers[seat.PassengerID] || !flights[seat.FlightID] {
				return errUnknownReference
			}
		}

		if err := tx.Where("passenger_id IN (?)",
			tx.Model(&data.IssuancePassenger{}).Select("id").Where("issuance_id = ?", issuanceID),
		).Delete(&data.IssuancePassengerFlight{}).Error; err != nil {
			return err
		}

		for _, seat := range req.Seats {
			row := data.IssuancePassengerFlight{
				PassengerID: seat.PassengerID,
				FlightID:    seat.FlightID,
				Seat:        blankToNil(seat.Seat),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errUnknownReference) {
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"Um passageiro ou trecho informado não pertence a esta emissão")
			return
		}
		slog.Error("failed to replace the seats of an issuance", "issuance_id", issuanceID, "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to save seats")
		return
	}

	h.broadcastIssuance("issuance.updated", issuanceID)
	utils.SendJSON(w, r, http.StatusOK, "Seats saved successfully", req.Seats)
}

// GetIssuanceBaggage handles the GET /issuances/{id}/baggage route.
// @Summary      Bagagens da emissão
// @Description  Devolve uma franquia por passageiro por direção, na mesma forma que o PUT aceita de volta.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {object}  schemas.IssuanceBaggageResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar bagagens"
// @Router       /issuances/{id}/baggage [get]
func (h *IssuanceHandler) GetIssuanceBaggage(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")
	if !h.issuanceExistsForServices(w, r, issuanceID) {
		return
	}

	var rows []data.IssuancePassengerBaggage
	if err := h.DB.
		Where("passenger_id IN (?)", h.passengerIDsOf(issuanceID)).
		Order("direction asc, position asc").
		Find(&rows).Error; err != nil {
		slog.Error("failed to read the baggage of an issuance", "issuance_id", issuanceID, "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch baggage")
		return
	}

	baggage := make([]schemas.IssuanceBaggage, 0, len(rows))
	for _, row := range rows {
		baggage = append(baggage, schemas.IssuanceBaggage{
			PassengerID: row.PassengerID,
			Direction:   row.Direction,
			Position:    row.Position,
			Weight:      row.Weight,
			Quantity:    row.Quantity,
		})
	}
	utils.SendJSON(w, r, http.StatusOK, "Baggage fetched successfully", baggage)
}

// PutIssuanceBaggage handles the PUT /issuances/{id}/baggage route.
// @Summary      Substituir as bagagens da emissão
// @Description  Substitui o conjunto inteiro de franquias numa transação. O corpo é o estado completo, nunca um delta.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                             true  "UUID da emissão"
// @Param        payload  body  schemas.PutIssuanceBaggageRequest   true  "Bagagens"
// @Success      200  {object}  schemas.IssuanceBaggageResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Passageiro não pertence a esta emissão, ou franquia duplicada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao gravar bagagens"
// @Router       /issuances/{id}/baggage [put]
func (h *IssuanceHandler) PutIssuanceBaggage(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")
	if !h.issuanceExistsForServices(w, r, issuanceID) {
		return
	}

	var req schemas.PutIssuanceBaggageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		passengers, _, err := ownedIDs(tx, issuanceID)
		if err != nil {
			return err
		}

		// The table has a UNIQUE (passenger_id, direction, position). Catching a
		// duplicate here answers 422 with a sentence; letting the constraint fire
		// answers 500 with a Postgres error string.
		type allowanceKey struct {
			passengerID string
			direction   string
			position    int
		}
		seen := make(map[allowanceKey]struct{}, len(req.Baggage))
		for _, allowance := range req.Baggage {
			if !passengers[allowance.PassengerID] {
				return errUnknownReference
			}
			key := allowanceKey{allowance.PassengerID, allowance.Direction, allowance.Position}
			if _, duplicate := seen[key]; duplicate {
				return errDuplicateAllowance
			}
			seen[key] = struct{}{}
		}

		if err := tx.Where("passenger_id IN (?)",
			tx.Model(&data.IssuancePassenger{}).Select("id").Where("issuance_id = ?", issuanceID),
		).Delete(&data.IssuancePassengerBaggage{}).Error; err != nil {
			return err
		}

		for _, allowance := range req.Baggage {
			row := data.IssuancePassengerBaggage{
				PassengerID: allowance.PassengerID,
				Direction:   allowance.Direction,
				Position:    allowance.Position,
				Weight:      blankToNil(allowance.Weight),
				Quantity:    allowance.Quantity,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errUnknownReference):
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"Um passageiro informado não pertence a esta emissão")
		case errors.Is(err, errDuplicateAllowance):
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"Há duas franquias na mesma posição para o mesmo passageiro e direção")
		default:
			slog.Error("failed to replace the baggage of an issuance", "issuance_id", issuanceID, "err", err)
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to save baggage")
		}
		return
	}

	h.broadcastIssuance("issuance.updated", issuanceID)
	utils.SendJSON(w, r, http.StatusOK, "Baggage saved successfully", req.Baggage)
}

var errDuplicateAllowance = errors.New("two allowances share a passenger, direction and position")

// issuanceExistsForServices answers 404 itself and reports whether to carry on.
func (h *IssuanceHandler) issuanceExistsForServices(w http.ResponseWriter, r *http.Request, issuanceID string) bool {
	err := h.DB.First(&data.Issuance{}, "id = ?", issuanceID).Error
	if err == nil {
		return true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return false
	}
	slog.Error("failed to load an issuance behind its services", "issuance_id", issuanceID, "err", err)
	utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch issuance")
	return false
}

// passengerIDsOf is a sub-query, not a slice: the caller only ever uses it
// inside an IN, and a round trip to fetch ids we immediately send back is a
// query for nothing.
func (h *IssuanceHandler) passengerIDsOf(issuanceID string) *gorm.DB {
	return h.DB.Model(&data.IssuancePassenger{}).Select("id").Where("issuance_id = ?", issuanceID)
}

// ownedIDs reads which passengers and flights belong to this issuance, so a
// payload cannot write a row onto somebody else's booking by naming its uuid.
func ownedIDs(tx *gorm.DB, issuanceID string) (passengers, flights map[string]bool, err error) {
	var passengerIDs, flightIDs []string
	if err := tx.Model(&data.IssuancePassenger{}).
		Where("issuance_id = ?", issuanceID).Pluck("id", &passengerIDs).Error; err != nil {
		return nil, nil, err
	}
	if err := tx.Model(&data.IssuanceFlight{}).
		Where("issuance_id = ?", issuanceID).Pluck("id", &flightIDs).Error; err != nil {
		return nil, nil, err
	}

	passengers = make(map[string]bool, len(passengerIDs))
	for _, id := range passengerIDs {
		passengers[id] = true
	}
	flights = make(map[string]bool, len(flightIDs))
	for _, id := range flightIDs {
		flights[id] = true
	}
	return passengers, flights, nil
}

// blankToNil folds an empty or whitespace-only string into nil. A cleared field
// in the panel arrives as "", and storing that would make "the airline gave no
// seat" and "the operator wiped the seat" two different values that render the
// same.
func blankToNil(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
