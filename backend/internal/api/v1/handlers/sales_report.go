package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/supplierreport"
	"github.com/murandido/m-80-backend/internal/utils"
)

// supplierReportBrand carries the per-account presentation bits that never reach the JSON
// response: the artwork and the colour behind the account's loyalty program.
type supplierReportBrand struct {
	programName    string
	programCode    string
	programColor   string
	defaultAirline *string
}

// GetSupplierReportPDF handles the GET /sales/suppliers/report route.
// @Summary      Resumo de pagamento ao fornecedor (PDF)
// @Description  Gera o PDF do resumo de emissões a pagar ao administrador da conta, com os mesmos filtros da tela de Fornecedores. Com account_id, sai o documento de uma conta; sem, sai uma seção por conta filtrada. As taxas entram no valor emissão a emissão, conforme taxes_included_in_cost de cada uma. Recusa com 422 se alguma emissão do documento estiver sem milheiro de custo. Rota restrita a administradores.
// @Tags         Vendas
// @Produce      application/pdf
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial do período (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final do período (YYYY-MM-DD), inclusiva"
// @Param        agency_id           query  string  false  "UUID da agência (padrão: a agência principal)"
// @Param        agency_ids          query  string  false  "UUIDs de agências separados por vírgula, ou 'all' para todas"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        account_id          query  string  false  "UUID da conta - restringe o documento a ela"
// @Param        administrator_id    query  string  false  "UUID do administrador da conta"
// @Success      200  {file}    file  "PDF do resumo"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário não é administrador"
// @Failure      422  {object}  schemas.ErrorResponse "Há emissões sem milheiro de custo"
// @Failure      502  {object}  schemas.ErrorResponse "Serviço de PDF indisponível"
// @Router       /sales/suppliers/report [get]
func (h *SaleHandler) GetSupplierReportPDF(w http.ResponseWriter, r *http.Request) {
	rows, from, to, fetchErr := h.fetchSupplierRows(r)
	if fetchErr != nil {
		utils.SendError(w, r, fetchErr.status, fetchErr.message)
		return
	}

	balances := aggregateSupplierBalances(rows, from, to)

	// The brand is looked up from the same rows the balances came from, so a document can
	// never be headed with a program the statement below it does not belong to.
	brands := map[string]supplierReportBrand{}
	for _, row := range rows {
		if _, seen := brands[row.AccountID]; !seen {
			brands[row.AccountID] = supplierReportBrand{
				programName:    row.LoyaltyProgramName,
				programCode:    row.LoyaltyProgramCode,
				programColor:   row.LoyaltyProgramColor,
				defaultAirline: row.DefaultAirlineCode,
			}
		}
	}

	model, missing := buildSupplierReport(balances, brands, supplierPeriodLabel(from, to))

	// Refusing is the point. A summary that silently drops - or zeroes - an issuance whose
	// cost was never agreed understates what the agency owes, and the supplier is the only
	// party with any reason to notice.
	if missing > 0 {
		utils.SendError(w, r, http.StatusUnprocessableEntity, supplierMissingCostMessage(missing))
		return
	}
	if len(model.Sections) == 0 {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Nenhuma emissão no período selecionado para gerar o resumo")
		return
	}

	document, err := supplierreport.Render(model)
	if err != nil {
		slog.Error("failed to render the supplier payment summary", "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to render the supplier summary")
		return
	}

	pdf, err := utils.GeneratePDF(document)
	if err != nil {
		slog.Error("failed to generate the supplier payment summary PDF", "err", err)
		utils.SendError(w, r, http.StatusBadGateway, "Não foi possível gerar o PDF do resumo")
		return
	}

	// Binary body, so no utils.SendJSON envelope - the same deliberate exception the xlsx
	// export and the ticket PDF make, for the same reason: there is no envelope in a PDF.
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", supplierReportFilename(model)))
	// #nosec G705 -- PDF bytes from our own pdf-service, served as an attachment
	// with an application/pdf content type; nothing here is interpreted as markup.
	if _, err := w.Write(pdf); err != nil {
		slog.Error("failed to write the supplier payment summary PDF", "err", err)
	}
}

// buildSupplierReport turns the balances into the document, and counts the issuances whose
// cost has not been agreed. It returns the count rather than an error so the caller can say
// how many are missing, which is the difference between a message an operator can act on
// and one they cannot.
func buildSupplierReport(
	balances []schemas.SupplierBalanceResponse,
	brands map[string]supplierReportBrand,
	periodLabel string,
) (supplierreport.ViewModel, int) {
	model := supplierreport.ViewModel{}
	grandTotal := 0
	missing := 0

	for _, balance := range balances {
		brand := brands[balance.AccountID]
		section := supplierreport.Section{
			AccountLabel: balance.AccountLabel,
			PeriodLabel:  periodLabel,
			ProgramName:  brand.programName,
			Brand:        brand.programColor,
			BrandOn:      supplierreport.ContrastColour(brand.programColor),
			Logo:         supplierreport.LogoFor(brand.programCode, brand.defaultAirline),
		}
		if balance.AdministratorName != nil {
			section.Administrator = *balance.AdministratorName
		}

		totalPoints, totalTaxes, totalAmount := 0, 0, 0
		for _, line := range balance.Issuances {
			if line.CostOfTheThousand == nil {
				missing++
				continue
			}

			amount := supplierLineAmount(line)
			totalPoints += line.Points
			totalAmount += amount
			// Only the taxes that were actually reimbursed are totalled, so the column
			// adds up to the part of "Total a pagar" it is responsible for.
			if line.TaxesIncludedInCost && line.Taxes != nil {
				totalTaxes += *line.Taxes
			}

			section.Lines = append(section.Lines, supplierreport.Line{
				Locator:     line.ProgramLocator,
				Date:        line.Date.In(supplierReportLocation()).Format("02/01/2006"),
				Origin:      derefString(line.OriginIATACode),
				Destination: derefString(line.DestinationIATACode),
				Points:      supplierreport.FormatPoints(line.Points),
				Milheiro:    supplierreport.FormatBRL(*line.CostOfTheThousand),
				Taxes:       supplierReportTaxes(line),
				Amount:      supplierreport.FormatBRL(amount),
				Refund:      line.Kind == "refund",
			})
		}

		// An account whose every line was skipped contributes no section - but `missing`
		// is already non-zero by then, so the request fails rather than producing a
		// document that quietly omits it.
		if len(section.Lines) == 0 {
			continue
		}

		section.TotalPoints = supplierreport.FormatPoints(totalPoints)
		section.TotalTaxes = supplierreport.FormatBRL(totalTaxes)
		section.TotalAmount = supplierreport.FormatBRL(totalAmount)
		grandTotal += totalAmount

		model.Sections = append(model.Sections, section)
	}

	return supplierreport.Finalize(model, grandTotal), missing
}

// supplierLineAmount is what the agency pays for one statement line, in centavos.
//
// Whether the taxes count is a property of the issuance rather than of the request: some
// administrators are reimbursed what they paid the airline and some are not, and the same
// account can hold both. That is why there is no flag to pass here - the document cannot
// disagree with the screen about it.
//
// The rounding is the same one the panel and the bot's own checksum use - points divided by
// a thousand, times the rate, rounded half up - so the figure on the screen, the figure in
// this PDF and the figure the operator typed all agree to the centavo.
func supplierLineAmount(line schemas.SupplierIssuanceResponse) int {
	amount := roundHalfUp(line.Points, *line.CostOfTheThousand)
	if line.TaxesIncludedInCost && line.Taxes != nil {
		amount += *line.Taxes
	}
	return amount
}

// supplierReportTaxes renders the taxes cell. A tax the agency is not reimbursing shows as
// a dash rather than as a figure: the document is sent to the supplier, and disclosing what
// was paid to the airline on a booking they are not being paid for it serves no purpose.
func supplierReportTaxes(line schemas.SupplierIssuanceResponse) string {
	if !line.TaxesIncludedInCost || line.Taxes == nil {
		return "—"
	}
	return supplierreport.FormatBRL(*line.Taxes)
}

// roundHalfUp computes points/1000 * rate in centavos without floating point, rounding half
// away from zero so a credit rounds the same distance as the booking it reverses.
func roundHalfUp(points, rate int) int {
	product := points * rate
	if product < 0 {
		return -((-product + 500) / 1000)
	}
	return (product + 500) / 1000
}

func supplierMissingCostMessage(missing int) string {
	if missing == 1 {
		return "1 emissão está sem milheiro de custo. Preencha antes de gerar o resumo."
	}
	return fmt.Sprintf("%d emissões estão sem milheiro de custo. Preencha antes de gerar o resumo.", missing)
}

// slugify reduces an account label to something safe inside a Content-Disposition filename.
// Anything that is not a letter, a digit or a dash becomes a dash, which also drops the
// quotes and semicolons that would otherwise let a label break out of the header.
func slugify(label string) string {
	var out strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				out.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(out.String(), "-")
	if slug == "" {
		return "fornecedor"
	}
	return slug
}

// supplierPeriodLabel renders the window the way the document's title reads it. An
// open-ended request has no period to name, so the title simply says so.
func supplierPeriodLabel(from, to *time.Time) string {
	loc := supplierReportLocation()
	switch {
	case from != nil && to != nil:
		// date_to arrives as the exclusive start of the next day; the label has to name
		// the inclusive last day the operator actually picked.
		last := to.Add(-time.Second).In(loc)
		first := from.In(loc)
		if first.Format("02/01/2006") == last.Format("02/01/2006") {
			return first.Format("02/01/2006")
		}
		return first.Format("02/01/2006") + " a " + last.Format("02/01/2006")
	case from != nil:
		return "a partir de " + from.In(loc).Format("02/01/2006")
	case to != nil:
		return "até " + to.Add(-time.Second).In(loc).Format("02/01/2006")
	default:
		return "todo o período"
	}
}

func supplierReportLocation() *time.Location {
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// supplierReportFilename names the download. One account gets its own name on the file, so
// a folder of summaries is sortable; several get a generic one.
func supplierReportFilename(model supplierreport.ViewModel) string {
	if len(model.Sections) != 1 {
		return "resumo-fornecedores.pdf"
	}
	return "resumo-" + slugify(model.Sections[0].AccountLabel) + ".pdf"
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
