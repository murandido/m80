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
// @Param        exclude_account_ids query  string  false  "UUIDs de contas separados por vírgula a excluir do documento"
// @Param        administrator_id    query  string  false  "UUID do administrador da conta"
// @Param        group_by            query  string  false  "Agrupamento das seções: account (padrão) ou administrator"
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

	balances := aggregateSupplierBalances(h.DB, rows, from, to, resolveSupplierGrouping(r))

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

	model, missing := buildSupplierReport(
		balances, brands, supplierPeriodLabel(from, to), resolveSupplierGrouping(r))

	// Refusing is the point. A summary that silently drops - or zeroes - an issuance whose
	// cost was never agreed understates what the agency owes, and the supplier is the only
	// party with any reason to notice.
	if missing > 0 {
		utils.SendError(w, r, http.StatusUnprocessableEntity, supplierMissingCostMessage(missing))
		return
	}
	if model.SectionCount() == 0 {
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
	// seq is the administrator's export counter, passed by the panel so the number it just
	// incremented lands in the file name. Absent (0) for the "all suppliers" document.
	seq, _ := strconv.Atoi(r.URL.Query().Get("seq"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", supplierReportFilename(model, seq, time.Now().In(supplierReportLocation()))))
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
	grouping supplierGrouping,
) (supplierreport.ViewModel, int) {
	model := supplierreport.ViewModel{}
	grand := supplierreport.Totals{}
	missing := 0

	// The recap is folded as the sections are built rather than in a pass of its own, so it
	// can only ever add up the very figures that were printed. Keyed by administrator, not
	// by group: grouped by administrator a group already is one, but grouped by account
	// several accounts land on the same person and have to meet somewhere.
	byAdministrator := map[string]*summaryAccumulator{}
	accounts := 0

	for _, balance := range balances {
		group := supplierreport.Group{}
		groupTotals := supplierreport.Totals{}
		adminKey, adminLabel := supplierAdministratorOf(balance)

		// Grouped by account the balance is already one statement. Grouped by administrator
		// it mixes their accounts, and it is split back apart here rather than upstream: a
		// section is what carries the programme colour and logo, and those are per account,
		// so one heading over the lot would have to pick a programme and be wrong.
		for _, part := range splitBalanceByAccount(balance, grouping) {
			brand := brands[part.accountID]
			section := supplierreport.Section{
				AccountLabel: part.accountLabel,
				PeriodLabel:  periodLabel,
				ProgramName:  brand.programName,
				Brand:        brand.programColor,
				BrandOn:      supplierreport.ContrastColour(brand.programColor),
				Logo:         supplierreport.LogoFor(brand.programCode, brand.defaultAirline),
			}
			if balance.AdministratorName != nil {
				section.Administrator = *balance.AdministratorName
			}

			totals := supplierreport.Totals{}
			for _, line := range part.lines {
				if line.CostOfTheThousand == nil {
					missing++
					continue
				}

				amount := supplierLineAmount(line)
				totals.Points += line.Points
				totals.AmountCents += amount
				// Only the taxes that were actually reimbursed are totalled, so the column
				// adds up to the part of "Total a pagar" it is responsible for.
				if line.TaxesIncludedInCost && line.Taxes != nil {
					totals.TaxesCents += *line.Taxes
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

				// The fee is what the supplier is paid for the CPFs the cancellation burned.
				// It follows its credit as a line of its own so the document shows why the
				// credit is smaller, and it adds to the amount only - it is neither miles
				// nor a tax.
				if line.CancelledCPFFee != nil && *line.CancelledCPFFee > 0 {
					totals.AmountCents += *line.CancelledCPFFee
					section.Lines = append(section.Lines, supplierreport.Line{
						Locator: line.ProgramLocator,
						Date:    line.Date.In(supplierReportLocation()).Format("02/01/2006"),
						Amount:  supplierreport.FormatBRL(*line.CancelledCPFFee),
						CPFFee:  true,
					})
				}
			}

			// An account whose every line was skipped contributes no section - but `missing`
			// is already non-zero by then, so the request fails rather than producing a
			// document that quietly omits it.
			if len(section.Lines) == 0 {
				continue
			}

			section.TotalPoints = supplierreport.FormatPoints(totals.Points)
			section.TotalTaxes = supplierreport.FormatBRL(totals.TaxesCents)
			section.TotalAmount = supplierreport.FormatBRL(totals.AmountCents)

			groupTotals.Points += totals.Points
			groupTotals.TaxesCents += totals.TaxesCents
			groupTotals.AmountCents += totals.AmountCents
			group.Sections = append(group.Sections, section)

			summary, known := byAdministrator[adminKey]
			if !known {
				summary = &summaryAccumulator{label: adminLabel, programs: map[string]string{}}
				byAdministrator[adminKey] = summary
			}
			summary.accounts++
			summary.totals.Points += totals.Points
			summary.totals.TaxesCents += totals.TaxesCents
			summary.totals.AmountCents += totals.AmountCents
			if brand.programName != "" {
				summary.programs[brand.programName] = brand.programColor
			}
			accounts++
		}

		if len(group.Sections) == 0 {
			continue
		}

		// Only a grouped document gets a heading and a subtotal. Left empty, the group is
		// just a container and the page renders exactly as it did before groups existed.
		if grouping == groupByAdministrator {
			group.Label = balance.GroupLabel
			group.TotalPoints = supplierreport.FormatPoints(groupTotals.Points)
			group.TotalTaxes = supplierreport.FormatBRL(groupTotals.TaxesCents)
			group.TotalAmount = supplierreport.FormatBRL(groupTotals.AmountCents)
		}

		grand.Points += groupTotals.Points
		grand.TaxesCents += groupTotals.TaxesCents
		grand.AmountCents += groupTotals.AmountCents
		model.Groups = append(model.Groups, group)
	}

	model.Summary = buildSupplierSummary(byAdministrator)
	model.SummaryAccountCount = supplierAccountCountLabel(accounts)

	return supplierreport.Finalize(model, grand), missing
}

// summaryAccumulator is one administrator's running total while the sections are built.
type summaryAccumulator struct {
	label    string
	accounts int
	// programs maps a loyalty program's name to its colour, so an administrator holding
	// three accounts in one program is badged once rather than three times.
	programs map[string]string
	totals   supplierreport.Totals
}

// supplierAdministratorOf answers which recap line a balance belongs to. It repeats the
// fallbacks of supplierGroupOf rather than sharing them because it starts from the response
// and not from a row - but the two have to agree, or the recap would name an administrator
// the group heading above it does not.
func supplierAdministratorOf(balance schemas.SupplierBalanceResponse) (key, label string) {
	if balance.AdministratorID == nil || *balance.AdministratorID == "" {
		return noAdministratorKey, "Sem administrador"
	}
	if balance.AdministratorName != nil && *balance.AdministratorName != "" {
		return *balance.AdministratorID, *balance.AdministratorName
	}
	return *balance.AdministratorID, *balance.AdministratorID
}

// buildSupplierSummary formats the recap, biggest debt first with the name as a tiebreak -
// the same order aggregateSupplierBalances puts the screen in, so the closing table reads
// in the order the operator already knows. The tiebreak is what makes two identical requests
// produce an identical document: map iteration order is not.
func buildSupplierSummary(byAdministrator map[string]*summaryAccumulator) []supplierreport.SummaryRow {
	ordered := make([]*summaryAccumulator, 0, len(byAdministrator))
	for _, summary := range byAdministrator {
		ordered = append(ordered, summary)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].totals.AmountCents != ordered[j].totals.AmountCents {
			return ordered[i].totals.AmountCents > ordered[j].totals.AmountCents
		}
		return ordered[i].label < ordered[j].label
	})

	rows := make([]supplierreport.SummaryRow, 0, len(ordered))
	for _, summary := range ordered {
		names := make([]string, 0, len(summary.programs))
		for name := range summary.programs {
			names = append(names, name)
		}
		sort.Strings(names)

		chips := make([]supplierreport.ProgramChip, 0, len(names))
		for _, name := range names {
			colour := summary.programs[name]
			chips = append(chips, supplierreport.ProgramChip{
				Name:    name,
				Brand:   colour,
				BrandOn: supplierreport.ContrastColour(colour),
			})
		}

		rows = append(rows, supplierreport.SummaryRow{
			Administrator: summary.label,
			Programs:      chips,
			AccountCount:  supplierAccountCountLabel(summary.accounts),
			Points:        supplierreport.FormatPoints(summary.totals.Points),
			Taxes:         supplierSummaryTaxes(summary.totals.TaxesCents),
			Amount:        supplierreport.FormatBRL(summary.totals.AmountCents),
		})
	}

	return rows
}

// supplierSummaryTaxes leaves the column empty-looking when nothing was reimbursed. A "R$
// 0,00" there reads as a figure that was checked and came to zero; the dash says the taxes
// were simply not part of what is being paid, which is what the statement lines above
// already say line by line.
func supplierSummaryTaxes(cents int) string {
	if cents == 0 {
		return "—"
	}
	return supplierreport.FormatBRL(cents)
}

func supplierAccountCountLabel(accounts int) string {
	if accounts == 1 {
		return "1 conta"
	}
	return fmt.Sprintf("%d contas", accounts)
}

// balancePart is one account's worth of a balance's statement.
type balancePart struct {
	accountID    string
	accountLabel string
	lines        []schemas.SupplierIssuanceResponse
}

// splitBalanceByAccount returns the statements a balance should become. Grouped by account
// that is the balance itself; grouped by administrator it is one per account behind it, in
// a stable order so two identical requests produce the same document.
func splitBalanceByAccount(
	balance schemas.SupplierBalanceResponse,
	grouping supplierGrouping,
) []balancePart {
	if grouping != groupByAdministrator {
		return []balancePart{{
			accountID:    balance.AccountID,
			accountLabel: balance.AccountLabel,
			lines:        balance.Issuances,
		}}
	}

	parts := make([]balancePart, 0, 2)
	index := map[string]int{}
	for _, line := range balance.Issuances {
		at, ok := index[line.AccountID]
		if !ok {
			at = len(parts)
			index[line.AccountID] = at
			parts = append(parts, balancePart{accountID: line.AccountID, accountLabel: line.AccountLabel})
		}
		parts[at].lines = append(parts[at].lines, line)
	}

	sort.Slice(parts, func(i, j int) bool { return parts[i].accountLabel < parts[j].accountLabel })
	return parts
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
//
// On an account that owns its points the rate is not the source: CostCents already holds
// what the batches charged, exactly, and the rate beside it is that figure rounded for
// display. Multiplying the rounded rate back out would disagree with the batch it came
// from by a centavo or so per booking, which over a month is a discrepancy with no line to
// blame it on.
func supplierLineAmount(line schemas.SupplierIssuanceResponse) int {
	amount := roundHalfUp(line.Points, *line.CostOfTheThousand)
	if line.CostCents != nil {
		amount = *line.CostCents
	}
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

// supplierReportFilename names the download. A per-administrator export carries its export
// sequence number (seq) so a stack of one administrator's summaries stays ordered and a folder
// of them is sortable; the "all suppliers" document, which spans several administrators, has no
// single sequence and is dated instead.
//
// The number has to be built here rather than by the panel's <a download>: browsers give the
// server's Content-Disposition filename precedence over the download attribute, even same-origin.
func supplierReportFilename(model supplierreport.ViewModel, seq int, now time.Time) string {
	suffix := ""
	if seq > 0 {
		suffix = fmt.Sprintf("-%04d", seq)
	}
	// A grouped document is named after the group even when it holds a single account, and
	// a run of accounts under one administrator has no single account to be named after.
	if len(model.Groups) == 1 && model.Groups[0].Label != "" {
		return "resumo-" + slugify(model.Groups[0].Label) + suffix + ".pdf"
	}
	if model.SectionCount() != 1 {
		return "resumo-fornecedores-" + now.Format("2006-01-02") + ".pdf"
	}
	return "resumo-" + slugify(model.Groups[0].Sections[0].AccountLabel) + suffix + ".pdf"
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
