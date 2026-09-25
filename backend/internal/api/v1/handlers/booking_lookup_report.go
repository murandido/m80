package handlers

import (
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The report reads a window of booking_lookup_attempts. TEMPORARY, with the table it
// reads - see migration 66: once no carrier falls back to Traveos there is nothing left
// to report, and this file, its route and its screen go.
const (
	defaultLookupReportDays = 7
	maxLookupReportDays     = 90
	maxRecentFailures       = 10
)

type BookingLookupReportHandler struct {
	DB       *gorm.DB
	Bookings *booking.Registry
}

func NewBookingLookupReportHandler(db *gorm.DB, bookings *booking.Registry) *BookingLookupReportHandler {
	return &BookingLookupReportHandler{DB: db, Bookings: bookings}
}

// lookupAttemptRow is an attempt with the locator the panel shows for its issuance.
type lookupAttemptRow struct {
	data.BookingLookupAttempt
	ProgramLocator string  `gorm:"column:program_locator"`
	AirlineLocator *string `gorm:"column:airline_locator"`
}

// GetReport handles the GET /booking-lookups/report route
// @Summary      Relatório das conexões diretas
// @Description  Por companhia aérea, quantas buscas de itinerário a integração direta respondeu, quantas caíram para a Traveos e por quê, numa janela de dias. Temporário: existe para decidir quando cada companhia pode deixar a Traveos.
// @Tags         Companhias Aéreas
// @Produce      json
// @Security     BearerAuth
// @Param        days  query  int  false  "Janela em dias (1 a 90, padrão 7)"
// @Success      200  {object}  schemas.DirectLookupReportResponse
// @Failure      400  {object}  schemas.ErrorResponse "Janela inválida"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao montar o relatório"
// @Router       /booking-lookups/report [get]
func (h *BookingLookupReportHandler) GetReport(w http.ResponseWriter, r *http.Request) {
	days := defaultLookupReportDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxLookupReportDays {
			utils.SendError(w, r, http.StatusBadRequest, "days must be between 1 and 90")
			return
		}
		days = parsed
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	var airlines []data.Airline
	if err := h.DB.Order("name").Find(&airlines).Error; err != nil {
		slog.Error("direct lookup report: reading airlines", "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to build the report")
		return
	}

	var rows []lookupAttemptRow
	if err := h.DB.Table("booking_lookup_attempts AS a").
		Select("a.*, i.program_locator, i.airline_locator").
		Joins("JOIN issuances i ON i.id = a.issuance_id").
		Where("a.created_at >= ?", since).
		Order("a.created_at DESC").
		Scan(&rows).Error; err != nil {
		slog.Error("direct lookup report: reading attempts", "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to build the report")
		return
	}

	fallbackName := ""
	if fallback := h.Bookings.Fallback(); fallback != nil {
		fallbackName = fallback.Name()
	}
	directName := func(code string) string {
		if provider, ok := h.Bookings.Direct(code); ok {
			return provider.Name()
		}
		return ""
	}

	utils.SendJSON(w, r, http.StatusOK, "Direct lookup report fetched successfully", schemas.DirectLookupReport{
		Days:     days,
		Since:    since,
		Airlines: buildLookupReport(airlines, rows, directName, fallbackName),
	})
}

// buildLookupReport aggregates the attempts per airline. An airline is listed when it has
// a registered integration or any attempt in the window; the rest have nothing to say.
// Kept apart from the handler so the counting is testable without a database.
func buildLookupReport(
	airlines []data.Airline,
	rows []lookupAttemptRow,
	directName func(code string) string,
	fallbackName string,
) []schemas.AirlineLookupReport {
	byAirline := map[string][]lookupAttemptRow{}
	for _, row := range rows {
		byAirline[row.AirlineID] = append(byAirline[row.AirlineID], row)
	}

	result := []schemas.AirlineLookupReport{}
	for _, airline := range airlines {
		attempts := byAirline[airline.ID]
		direct := directName(airline.Code)
		if direct == "" && len(attempts) == 0 {
			continue
		}
		line := schemas.AirlineLookupReport{
			AirlineID:            airline.ID,
			Code:                 airline.Code,
			Name:                 airline.Name,
			DirectBookingEnabled: airline.DirectBookingEnabled,
			TraveosSupported:     airline.TraveosSupported,
			DirectProvider:       direct,
			RecentFailures:       []schemas.LookupFailure{},
		}

		// Group by sync, remembering the order the syncs arrived in (newest first).
		bySync := map[string][]lookupAttemptRow{}
		var syncOrder []string
		for _, attempt := range attempts {
			if _, seen := bySync[attempt.SyncID]; !seen {
				syncOrder = append(syncOrder, attempt.SyncID)
			}
			bySync[attempt.SyncID] = append(bySync[attempt.SyncID], attempt)
		}

		var directMS, directCount int
		for _, syncID := range syncOrder {
			sync := bySync[syncID]
			line.Syncs++

			answeredBy, askedDirect := "", false
			for _, attempt := range sync {
				if attempt.Outcome == data.LookupFound {
					answeredBy = attempt.Provider
				}
				if attempt.Provider == fallbackName {
					continue
				}
				askedDirect = true
				directCount++
				directMS += attempt.DurationMS
				switch attempt.Outcome {
				case data.LookupFound:
					line.DirectFound++
				case data.LookupNotFound:
					line.DirectNotFound++
				case data.LookupUnavailable:
					line.DirectUnavailable++
				default:
					line.DirectRefused++
				}
			}

			switch answeredBy {
			case "":
				line.Unanswered++
			case fallbackName:
				line.AnsweredByTraveos++
				if askedDirect {
					line.FellBackToTraveos++
				}
			default:
				line.AnsweredByDirect++
			}

			for _, attempt := range sync {
				if attempt.Provider == fallbackName || attempt.Outcome == data.LookupFound ||
					len(line.RecentFailures) >= maxRecentFailures {
					continue
				}
				line.RecentFailures = append(line.RecentFailures, lookupFailure(attempt, answeredBy))
			}
		}
		if directCount > 0 {
			line.DirectAverageMS = directMS / directCount
		}
		result = append(result, line)
	}

	// Airlines with something to look at first: the most syncs, then by name.
	sort.SliceStable(result, func(i, j int) bool { return result[i].Syncs > result[j].Syncs })
	return result
}

func lookupFailure(attempt lookupAttemptRow, answeredBy string) schemas.LookupFailure {
	locator := attempt.ProgramLocator
	if attempt.AirlineLocator != nil && *attempt.AirlineLocator != "" {
		locator = *attempt.AirlineLocator
	}
	message := ""
	if attempt.Error != nil {
		message = *attempt.Error
	}
	return schemas.LookupFailure{
		IssuanceID: attempt.IssuanceID,
		Locator:    locator,
		Outcome:    attempt.Outcome,
		Error:      message,
		AnsweredBy: answeredBy,
		CreatedAt:  attempt.CreatedAt,
	}
}
