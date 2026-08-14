package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/legacyimport"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The pre-panel spreadsheet is around 600 KB for a month of thirteen days. The ceiling is
// generous against that and still small enough that no request can make excelize chew
// through an arbitrary amount of memory - this route is reachable from a public domain,
// and a spreadsheet parser is a decompressor with extra steps.
const legacyImportMaxUpload = 8 << 20 // 8 MiB

// legacyImportMaxRowsPerDay bounds what one commit can write. Real days run to 45 rows; a
// file claiming thousands is either not this spreadsheet or is trying to make the API
// write a lot of issuances in one transaction.
const legacyImportMaxRowsPerDay = 500

// LegacyImportHandler serves the temporary screen that brings the hand-kept
// "Controle de Emissões" spreadsheet into the database. It goes away with the rest of the
// importer once the migration off spreadsheets is done - see internal/legacyimport.
type LegacyImportHandler struct {
	DB     *gorm.DB
	Broker *events.Broker
}

func NewLegacyImportHandler(db *gorm.DB, broker *events.Broker) *LegacyImportHandler {
	return &LegacyImportHandler{DB: db, Broker: broker}
}

// PreviewLegacyImport handles POST /issuances/legacy-import/preview
// @Summary      Prévia da importação da planilha manual
// @Description  Lê a planilha enviada e devolve os dias que ela contém. Com o campo day (YYYY-MM-DD), devolve também a prévia daquele dia: cada linha já resolvida em ids, com o veredito de importação, mais os apelidos de conta e os nomes de emissor que faltam mapear. Nada é gravado. mappings é o JSON com as escolhas feitas na tela, para a prévia refletir o que o commit faria.
// @Tags         Emissões
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file     formData  file    true   "Planilha .xlsx"
// @Param        day      formData  string  false  "Dia a detalhar (YYYY-MM-DD)"
// @Param        mappings formData  string  false  "JSON com as escolhas de conta, emissor e agência"
// @Success      200  {object}  schemas.LegacyImportPreviewSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Arquivo ausente, grande demais ou ilegível"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não é admin"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao ler a planilha"
// @Router       /issuances/legacy-import/preview [post]
func (h *LegacyImportHandler) PreviewLegacyImport(w http.ResponseWriter, r *http.Request) {
	days, mappings, ok := h.readRequest(w, r)
	if !ok {
		return
	}

	response := schemas.LegacyImportPreviewResponse{Days: summariseDays(days)}

	if rawDay := r.FormValue("day"); rawDay != "" {
		day, found := findDay(days, rawDay)
		if !found {
			utils.SendError(w, r, http.StatusBadRequest, "A planilha não tem uma aba para esse dia")
			return
		}

		preview, err := legacyimport.Resolve(h.DB, day, mappings)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError,
				"Não consegui resolver as linhas do dia: "+err.Error())
			return
		}
		response.Preview = &preview
	}

	utils.SendJSON(w, r, http.StatusOK, "Legacy import preview built", response)
}

// CommitLegacyImport handles POST /issuances/legacy-import/commit
// @Summary      Importar um dia da planilha manual
// @Description  Grava as linhas marcadas de um dia da planilha. O arquivo é lido e resolvido de novo do zero - a prévia é conselho, não contrato -, então o cliente manda quais linhas quer (rows), nunca o conteúdo delas. Linhas que não resolvem, ou cujo localizador já existe, são puladas com o motivo em vez de derrubar o dia inteiro. As emissões nascem incompletas, com created_at no dia da aba e imported=true.
// @Tags         Emissões
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file     formData  file    true   "Planilha .xlsx"
// @Param        day      formData  string  true   "Dia a importar (YYYY-MM-DD)"
// @Param        mappings formData  string  false  "JSON com as escolhas de conta, emissor e agência"
// @Param        rows     formData  string  true   "JSON com as linhas marcadas ([{block_index, number}])"
// @Success      200  {object}  schemas.LegacyImportCommitSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Arquivo, dia ou seleção inválidos"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Não é admin"
// @Failure      422  {object}  schemas.ErrorResponse "Seleção maior que o teto de linhas por dia"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao importar"
// @Router       /issuances/legacy-import/commit [post]
func (h *LegacyImportHandler) CommitLegacyImport(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	days, mappings, ok := h.readRequest(w, r)
	if !ok {
		return
	}

	day, found := findDay(days, r.FormValue("day"))
	if !found {
		utils.SendError(w, r, http.StatusBadRequest, "A planilha não tem uma aba para esse dia")
		return
	}

	var selected []legacyimport.RowSelection
	if err := json.Unmarshal([]byte(r.FormValue("rows")), &selected); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Seleção de linhas inválida")
		return
	}
	if len(selected) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "Nenhuma linha marcada para importar")
		return
	}
	if len(selected) > legacyImportMaxRowsPerDay {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			fmt.Sprintf("Um dia não pode importar mais de %d linhas", legacyImportMaxRowsPerDay))
		return
	}

	result, err := legacyimport.Commit(h.DB, day, mappings, selected, userCtx.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui importar o dia: "+err.Error())
		return
	}

	// One event for the whole day rather than one per issuance: every panel refetches
	// the list with its own filters anyway, and a few hundred broadcasts in a row would
	// be a few hundred refetches in every open browser.
	if result.Imported > 0 {
		h.broadcastIssuance(eventIssuanceCreated, "")
	}

	utils.SendJSON(w, r, http.StatusOK, "Legacy import committed", result)
}

// broadcastIssuance mirrors the issuance handler's own announcement. The payload carries
// no id because an import is a batch: the clients refetch, which is all they do with the
// id anyway.
func (h *LegacyImportHandler) broadcastIssuance(eventType, issuanceID string) {
	h.Broker.Broadcast(events.Event{
		Type:    eventType,
		Payload: map[string]string{"id": issuanceID},
	})
}

// readRequest does the part both routes share: read the upload, parse it, and read the
// operator's mappings. It answers the client itself on failure and reports whether the
// caller should carry on.
func (h *LegacyImportHandler) readRequest(
	w http.ResponseWriter,
	r *http.Request,
) ([]legacyimport.Day, legacyimport.Mappings, bool) {
	var mappings legacyimport.Mappings

	r.Body = http.MaxBytesReader(w, r.Body, legacyImportMaxUpload)
	//nolint:gosec // G120: the body is bounded on the line above - MaxBytesReader caps the
	// total read, so the parse cannot pull in more than legacyImportMaxUpload however the
	// parts are laid out. gosec does not follow the wrap through this helper's parameters.
	if err := r.ParseMultipartForm(legacyImportMaxUpload); err != nil {
		utils.SendError(w, r, http.StatusBadRequest,
			"Não consegui ler o envio. A planilha precisa ter menos de 8 MB")
		return nil, mappings, false
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Envie a planilha no campo \"file\"")
		return nil, mappings, false
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Não consegui ler o arquivo enviado")
		return nil, mappings, false
	}

	days, err := legacyimport.Parse(content)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest,
			"O arquivo não parece ser a planilha de controle de emissões")
		return nil, mappings, false
	}
	if len(days) == 0 {
		utils.SendError(w, r, http.StatusBadRequest,
			"A planilha não tem nenhuma aba de dia (as abas de dia se chamam DD-MM-AA)")
		return nil, mappings, false
	}

	if raw := r.FormValue("mappings"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &mappings); err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Mapeamentos inválidos")
			return nil, mappings, false
		}
	}

	return days, mappings, true
}

// findDay picks the sheet for a "YYYY-MM-DD" day. The client sends the date rather than
// the sheet name because the date is what it was given, and the sheet name is a detail of
// how the spreadsheet happens to be organised.
func findDay(days []legacyimport.Day, raw string) (legacyimport.Day, bool) {
	wanted, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return legacyimport.Day{}, false
	}
	for _, day := range days {
		if day.Date.Equal(wanted) {
			return day, true
		}
	}
	return legacyimport.Day{}, false
}

func summariseDays(days []legacyimport.Day) []schemas.LegacyImportDaySummary {
	summaries := make([]schemas.LegacyImportDaySummary, 0, len(days))
	for _, day := range days {
		rows := 0
		for _, block := range day.Blocks {
			rows += len(block.Rows)
		}
		summaries = append(summaries, schemas.LegacyImportDaySummary{
			Date:      day.Date.Format("2006-01-02"),
			SheetName: day.SheetName,
			RowCount:  rows,
			Blocks:    len(day.Blocks),
		})
	}
	return summaries
}
