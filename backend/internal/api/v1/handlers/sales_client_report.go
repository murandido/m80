package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/clientreport"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/supplierreport"
	"github.com/murandido/m-80-backend/internal/utils"
)

// GetClientReportPDF handles the GET /sales/clients/report route.
// @Summary      Controle de pagamentos do cliente (PDF)
// @Description  Gera o PDF que a agência envia ao cliente: as emissões do período com valor, quanto já foi pago, quanto falta e a situação de cada uma. Aceita os mesmos filtros da tela de Controle de pagamentos; sai uma seção por cliente, e as vendas sem cliente numa seção própria. Responde 422 se não houver emissão no filtro.
// @Tags         Vendas
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        date_from   query  string  false  "Data inicial da emissão (YYYY-MM-DD), inclusiva"
// @Param        date_to     query  string  false  "Data final da emissão (YYYY-MM-DD), inclusiva"
// @Param        agency_id   query  string  false  "UUID da agência (padrão: a agência principal)"
// @Param        client_ids  query  string  false  "UUIDs de clientes separados por vírgula; 'none' inclui as vendas sem cliente"
// @Param        paid        query  boolean false  "Filtra por vendas pagas (true) ou em aberto (false)"
// @Param        q           query  string  false  "Busca por localizador, passageiro, origem ou destino da emissão"
// @Success      200  {file}    file  "PDF do controle"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Nenhuma emissão no filtro"
// @Failure      502  {object}  schemas.ErrorResponse "Serviço de PDF indisponível"
// @Router       /sales/clients/report [get]
func (h *SaleHandler) GetClientReportPDF(w http.ResponseWriter, r *http.Request) {
	agencyID, err := resolveAgencyFilter(h.DB, r)
	if err != nil {
		sendAgencyFilterError(w, r, err)
		return
	}

	// The listing's own query, filters and order: the statement a client receives has to be
	// the list the operator was looking at when they pressed the button.
	var rows []saleRow
	err = applySaleListFilters(saleListQuery(h.DB).Where("issuances.agency_id = ?", agencyID), r).
		Order("issuances.created_at asc, sales.id").
		Scan(&rows).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch sales")
		return
	}
	if len(rows) == 0 {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Nenhuma emissão no filtro selecionado para gerar o controle")
		return
	}

	// Shaped like the listing's rows, so the statement shows on an alteration day what the
	// screen shows there.
	from, to := parseDateRange(r)
	alterations := alterationCentsInWindow(h.DB, rows, from, to)

	sales := make([]schemas.SaleResponse, 0, len(rows))
	credits := map[string]int{}
	for _, row := range rows {
		sales = append(sales, asAlterationDay(toSaleResponse(row), alterations, from, to))
		if row.ClientID != nil {
			credits[*row.ClientID] = 0
		}
	}
	for clientID := range credits {
		if credits[clientID], err = sale.CreditOf(h.DB, clientID); err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client credit")
			return
		}
	}

	var agency data.Agency
	if err := h.DB.Select("name").First(&agency, "id = ?", agencyID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load the agency")
		return
	}

	now := time.Now().In(supplierReportLocation())
	model := buildClientReport(sales, credits, agency.Name, supplierPeriodLabel(from, to), now)

	document, err := clientreport.Render(model)
	if err != nil {
		slog.Error("failed to render the client payment statement", "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to render the client statement")
		return
	}

	pdf, err := utils.GeneratePDF(document)
	if err != nil {
		slog.Error("failed to generate the client payment statement PDF", "err", err)
		utils.SendError(w, r, http.StatusBadGateway, "Não foi possível gerar o PDF do controle")
		return
	}

	// Binary body, so no utils.SendJSON envelope - the same exception the supplier summary
	// makes, for the same reason: there is no envelope in a PDF.
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", clientReportFilename(model, now)))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	// #nosec G705 -- PDF bytes from our own pdf-service, served as an attachment
	// with an application/pdf content type; nothing here is interpreted as markup.
	if _, err := w.Write(pdf); err != nil {
		slog.Error("failed to write the client payment statement PDF", "err", err)
	}
}

// noClientSectionName heads the sales nobody was linked to - the same words the filter uses.
const noClientSectionName = "Sem cliente registrado"

// buildClientReport groups the sales by client, in name order with the unlinked ones last,
// and formats every figure. The sales arrive in the listing's order, oldest first, and keep
// it inside each section.
func buildClientReport(
	sales []schemas.SaleResponse,
	credits map[string]int,
	agencyName, periodLabel string,
	now time.Time,
) clientreport.ViewModel {
	type accumulator struct {
		clientID string
		section  clientreport.Section
		amount   int
		paid     int
		balance  int
	}
	byClient := map[string]*accumulator{}
	order := []*accumulator{}

	loc := supplierReportLocation()
	for _, s := range sales {
		key, name := "", noClientSectionName
		if s.ClientID != nil {
			key = *s.ClientID
			if s.ClientName != nil {
				name = *s.ClientName
			}
		}
		acc, seen := byClient[key]
		if !seen {
			acc = &accumulator{clientID: key, section: clientreport.Section{ClientName: name}}
			byClient[key] = acc
			order = append(order, acc)
		}

		status, kind := clientReportStatus(s)
		cancelled := s.CancelledAt != nil
		passenger := ""
		if s.MainPassengerName != nil {
			passenger = *s.MainPassengerName
		}
		acc.section.Lines = append(acc.section.Lines, clientreport.Line{
			Date:       s.IssuedAt.In(loc).Format("02/01/2006"),
			Locator:    s.ProgramLocator,
			Passenger:  passenger,
			Amount:     supplierreport.FormatBRL(s.AmountCents),
			Paid:       supplierreport.FormatBRL(s.PaidCents),
			Balance:    supplierreport.FormatBRL(s.BalanceCents),
			Status:     status,
			StatusKind: kind,
			Cancelled:  cancelled,
		})
		// A cancelled ticket is a debt only for its cancellation fee: its price stays out of
		// the total bought, the fee goes in, and what was paid for it and what has to go
		// back still count.
		if cancelled {
			acc.amount += s.CancellationFeeCents
		} else {
			acc.amount += s.AmountCents
		}
		acc.paid += s.PaidCents
		acc.balance += s.BalanceCents
	}

	sort.SliceStable(order, func(i, j int) bool {
		if (order[i].clientID == "") != (order[j].clientID == "") {
			return order[j].clientID == ""
		}
		return order[i].section.ClientName < order[j].section.ClientName
	})

	model := clientreport.ViewModel{
		AgencyName:  agencyName,
		PeriodLabel: periodLabel,
		GeneratedAt: now.Format("02/01/2006 15:04"),
		Sections:    make([]clientreport.Section, 0, len(order)),
	}
	for _, acc := range order {
		section := acc.section
		section.TotalAmount = supplierreport.FormatBRL(acc.amount)
		section.TotalPaid = supplierreport.FormatBRL(acc.paid)
		section.TotalBalance = supplierreport.FormatBRL(acc.balance)
		section.Settled = acc.balance <= 0
		if credit := credits[acc.clientID]; acc.clientID != "" && credit > 0 {
			section.Credit = supplierreport.FormatBRL(credit)
		}
		model.Sections = append(model.Sections, section)
	}
	return model
}

// clientReportStatus mirrors saleStatus on the panel (sale-status-button.tsx): the statement
// has to call each issuance what the screen calls it.
func clientReportStatus(s schemas.SaleResponse) (label, kind string) {
	switch {
	case s.CancelledAt != nil && s.BalanceCents < 0:
		return "Devolver", "refund"
	case s.CancelledAt != nil && s.BalanceCents > 0:
		// The cancellation fee, still to be paid.
		return "Multa a pagar", "unpaid"
	case s.CancelledAt != nil:
		return "Cancelada", "cancelled"
	case s.AmountCents <= 0:
		return "Sem valor", "novalue"
	case s.BalanceCents <= 0:
		return "Pago", "paid"
	case s.PaidCents > 0:
		return "Parcial", "partial"
	default:
		return "A pagar", "unpaid"
	}
}

// clientReportFilename names the download after the client when the statement is one
// client's, which is the one that gets sent, and after the date otherwise.
func clientReportFilename(model clientreport.ViewModel, now time.Time) string {
	day := now.Format("2006-01-02")
	if len(model.Sections) == 1 && model.Sections[0].ClientName != noClientSectionName {
		return "controle-" + slugify(foldAccents(model.Sections[0].ClientName)) + "-" + day + ".pdf"
	}
	return "controle-pagamentos-" + day + ".pdf"
}

// accentFolder turns the accented letters of Portuguese names into their plain forms, so
// slugify - which keeps only a-z and digits - names "JOÃO" joao instead of jo-o.
var accentFolder = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e", "ë", "e",
	"í", "i", "î", "i", "ì", "i", "ï", "i",
	"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o",
	"ú", "u", "û", "u", "ù", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

func foldAccents(name string) string {
	return accentFolder.Replace(strings.ToLower(name))
}
