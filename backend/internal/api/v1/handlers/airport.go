package handlers

import (
	"net/http"
	"sort"
	"strings"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

const airportSearchLimit = 20

type AirportHandler struct {
	DB *gorm.DB
}

func NewAirportHandler(db *gorm.DB) *AirportHandler {
	return &AirportHandler{DB: db}
}

func toAirportResponse(airport data.Airport) schemas.AirportResponse {
	return schemas.AirportResponse{
		ID:       airport.ID,
		IATACode: airport.IATACode,
		Name:     airport.Name,
		City:     airport.City,
		State:    airport.State,
		Country:  airport.Country,
	}
}

// GetAirports handles the GET /airports route.
// @Summary      Buscar aeroportos
// @Description  Busca aeroportos por código IATA, nome ou cidade (contains, case-insensitive), priorizando quem começa com o texto buscado. Sem "q", retorna uma lista vazia - a tabela tem milhares de linhas e não é servida inteira.
// @Tags         Aeroportos
// @Produce      json
// @Security     BearerAuth
// @Param        q  query  string  false  "Texto de busca (código IATA, nome ou cidade)"
// @Success      200  {object}  schemas.AirportListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar aeroportos"
// @Router       /airports [get]
func (h *AirportHandler) GetAirports(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	airports := make([]data.Airport, 0)
	if q != "" {
		like := "%" + q + "%"
		if err := h.DB.
			Where("iata_code ILIKE ? OR name ILIKE ? OR city ILIKE ?", like, like, like).
			Order("iata_code ASC").
			Limit(airportSearchLimit * 5).
			Find(&airports).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch airports")
			return
		}

		lowerQ := strings.ToLower(q)
		sort.SliceStable(airports, func(i, j int) bool {
			iStarts := strings.HasPrefix(strings.ToLower(airports[i].IATACode), lowerQ)
			jStarts := strings.HasPrefix(strings.ToLower(airports[j].IATACode), lowerQ)
			return iStarts && !jStarts
		})
		if len(airports) > airportSearchLimit {
			airports = airports[:airportSearchLimit]
		}
	}

	resp := make([]schemas.AirportResponse, 0, len(airports))
	for _, airport := range airports {
		resp = append(resp, toAirportResponse(airport))
	}

	utils.SendJSON(w, r, http.StatusOK, "Airports fetched successfully", resp)
}
