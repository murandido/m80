package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/ticket"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// GetIssuanceTicket handles the GET /issuances/{id}/ticket route.
// @Summary      Bilhete da emissão em HTML
// @Description  Devolve o bilhete do passageiro renderizado em HTML. É exatamente o mesmo documento que vira PDF em /issuances/{id}/pdf, então a página do painel é o preview fiel do arquivo. Exige uma emissão já sincronizada com a Traveos.
// @Tags         Emissões
// @Produce      html
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {string}  string  "Documento HTML do bilhete"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Emissão sem dados da Traveos"
// @Router       /issuances/{id}/ticket [get]
func (h *IssuanceHandler) GetIssuanceTicket(w http.ResponseWriter, r *http.Request) {
	document, _, ok := h.renderTicket(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// #nosec G705 -- the document is html/template output, which escapes every
	// interpolated value by construction; the only unescaped fragment in it is the
	// airline logo, and that comes from internal/ticket's own embedded SVG files.
	if _, err := w.Write([]byte(document)); err != nil {
		slog.Error("failed to write the ticket document", "err", err)
	}
}

// GetIssuanceTicketPDF handles the GET /issuances/{id}/pdf route.
// @Summary      Bilhete da emissão em PDF
// @Description  Passa o mesmo HTML de /issuances/{id}/ticket pelo pdf-service (WeasyPrint) e devolve o arquivo como anexo.
// @Tags         Emissões
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {file}  binary  "Arquivo PDF do bilhete"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Emissão sem dados da Traveos"
// @Failure      502  {object}  schemas.ErrorResponse "O serviço de PDF falhou"
// @Router       /issuances/{id}/pdf [get]
func (h *IssuanceHandler) GetIssuanceTicketPDF(w http.ResponseWriter, r *http.Request) {
	document, locator, ok := h.renderTicket(w, r)
	if !ok {
		return
	}

	pdf, err := utils.GeneratePDF(document)
	if err != nil {
		// 502 rather than 500: the failure is in a service behind this one, and
		// the distinction is what tells an operator to look at pdf-service instead
		// of at the issuance.
		slog.Error("failed to generate the ticket pdf", "locator", locator, "err", err)
		utils.SendError(w, r, http.StatusBadGateway, "Não foi possível gerar o PDF do bilhete")
		return
	}

	// The filename is what the passenger ends up with in their downloads folder:
	// the locator alone, exactly as Traveos names its own file. No "bilhete-"
	// prefix - the operator searches their downloads by PNR, and a prefix pushes
	// every ticket ever downloaded under the same letter.
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", locator+".pdf"))
	// #nosec G705 -- PDF bytes from our own pdf-service, served as an attachment
	// with an application/pdf content type; nothing here is interpreted as markup.
	if _, err := w.Write(pdf); err != nil {
		slog.Error("failed to write the ticket pdf", "err", err)
	}
}

// renderTicket loads the issuance and builds the document both routes serve.
// It writes the error response itself and reports whether the caller should
// carry on; the locator it returns names the downloaded file.
func (h *IssuanceHandler) renderTicket(w http.ResponseWriter, r *http.Request) (document, locator string, ok bool) {
	id := chi.URLParam(r, "id")

	issuance, seats, err := loadIssuanceForTicket(h.DB, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
			return "", "", false
		}
		slog.Error("failed to load the issuance behind a ticket", "issuance_id", id, "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch issuance")
		return "", "", false
	}

	// traveos_synced_at, not complete. `complete` can be ticked by hand on the
	// grid, and a ticket built from a row somebody ticked would have no flights
	// and no passengers on it - a blank document handed to a passenger.
	if issuance.TraveosSyncedAt == nil {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Essa emissão ainda não tem os dados da Traveos, então não há bilhete para mostrar")
		return "", "", false
	}

	document, err = ticket.Render(ticket.FromIssuance(issuance, seats))
	if err != nil {
		slog.Error("failed to render the ticket", "issuance_id", id, "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to render ticket")
		return "", "", false
	}

	locator = strings.TrimSpace(issuance.ProgramLocator)
	if issuance.AirlineLocator != nil && strings.TrimSpace(*issuance.AirlineLocator) != "" {
		locator = strings.TrimSpace(*issuance.AirlineLocator)
	}
	return document, locator, true
}

// loadIssuanceForTicket fetches everything the ticket draws in two queries: the
// issuance with its airline, flights (both directions, with airport names) and
// passengers, then the seat/baggage join rows for those passengers.
//
// The join rows are fetched separately and handed to the ticket package as a
// map rather than preloaded through the passengers, because GORM would emit one
// query per passenger for a nested has-many - four passengers on a round trip is
// then nine queries for one page.
func loadIssuanceForTicket(db *gorm.DB, id string) (data.Issuance, map[string][]data.IssuancePassengerFlight, error) {
	var issuance data.Issuance

	err := db.
		Preload("Airline").
		Preload("Flights", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("direction asc, leg_order asc")
		}).
		Preload("Flights.OriginAirport").
		Preload("Flights.DestinationAirport").
		Preload("Passengers", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("position asc")
		}).
		First(&issuance, "id = ?", id).Error
	if err != nil {
		return issuance, nil, err
	}

	passengerIDs := make([]string, 0, len(issuance.Passengers))
	for _, passenger := range issuance.Passengers {
		passengerIDs = append(passengerIDs, passenger.ID)
	}

	seats := make(map[string][]data.IssuancePassengerFlight, len(passengerIDs))
	if len(passengerIDs) == 0 {
		return issuance, seats, nil
	}

	var rows []data.IssuancePassengerFlight
	if err := db.Where("passenger_id IN ?", passengerIDs).Find(&rows).Error; err != nil {
		return issuance, nil, err
	}
	for _, row := range rows {
		seats[row.PassengerID] = append(seats[row.PassengerID], row)
	}
	return issuance, seats, nil
}
