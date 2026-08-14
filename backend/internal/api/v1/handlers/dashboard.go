package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	DB *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{DB: db}
}

// defaultGranularity is what an unrecognised (or absent) granularity falls back to. Like
// the date filters, a bad value widens into something sensible instead of 400-ing.
const defaultGranularity = "day"

// dashboardGranularities is the allowlist for the bucket size. It is a fixed set rather
// than a free string because the value drives date arithmetic, and an unknown one would
// silently produce a single meaningless bucket.
var dashboardGranularities = map[string]bool{
	"hour":  true,
	"day":   true,
	"week":  true,
	"month": true,
}

// dashboardIssuanceRow is one issuance's contribution to the dashboard. Like the suppliers
// view, rows are narrowed in SQL but folded in Go: the period rule is "counted where it was
// created, and again where it was cancelled", which as a SQL GROUP BY would mean repeating
// the same CASE expression across four rollups. One pure function is easier to test and
// impossible to let drift between them.
type dashboardIssuanceRow struct {
	Points             int
	ValueOfTheThousand int
	Taxes              *int
	RefundedPoints     *int
	CreatedAt          time.Time
	CancelledAt        *time.Time

	LoyaltyProgramID    string
	LoyaltyProgramName  string
	LoyaltyProgramColor string
	IssuerID            string
	IssuerName          string
}

// amountCents is what this issuance bills: miles at the agreed price per thousand, plus
// taxes. Shares pointsValueCents with the sales views so the dashboard and the "Vendas"
// table never disagree by a cent on the same issuance.
func (row dashboardIssuanceRow) amountCents() int {
	amount := sale.PointsValueCents(row.Points, row.ValueOfTheThousand)
	if row.Taxes != nil {
		amount += *row.Taxes
	}
	return amount
}

// dashboardIncludesAmount says whether a role gets to see money on the dashboard. Kept as
// its own function so the rule can be tested without a database - the handler that uses it
// cannot be.
func dashboardIncludesAmount(role string) bool {
	return role == data.RoleAdmin || role == data.RoleDev
}

// GetDashboard handles the GET /dashboard route.
// @Summary      Métricas agregadas do dashboard
// @Description  Agrega as emissões do período em totais, série temporal, e rollups por programa de fidelidade e por emissor. Sem agency_id, filtra pela agência principal (is_default=true). Emissões canceladas ficam fora de todo o volume (contagem, pontos, valor e rollups), em qualquer período — mesmo que o cancelamento tenha ocorrido fora da janela consultada; elas aparecem apenas em cancelled_count e refunded_points, pela data do cancelamento. O campo amount_cents é omitido para o usuário comum; administradores e desenvolvedores veem os valores.
// @Tags         Dashboard
// @Produce      json
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial do período (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final do período (YYYY-MM-DD), inclusiva"
// @Param        granularity         query  string  false  "Tamanho do bucket da série temporal" Enums(hour, day, week, month) default(day)
// @Param        agency_id           query  string  false  "UUID da agência (padrão: a agência principal)"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        airline_id          query  string  false  "UUID da cia aérea"
// @Param        issuer_id           query  string  false  "UUID do emissor (usuário)"
// @Success      200  {object}  schemas.DashboardSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Nenhuma agência principal configurada ou erro ao calcular o dashboard"
// @Router       /dashboard [get]
func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	agencyID, err := resolveAgencyFilter(h.DB, r)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "No default agency configured")
		return
	}

	// Money is decided from the user the auth middleware loaded from the database, never
	// from a query param or a client-supplied header.
	user, _ := r.Context().Value(middleware.UserContextKey).(data.User)
	includeAmount := dashboardIncludesAmount(user.Role)

	granularity := r.URL.Query().Get("granularity")
	if !dashboardGranularities[granularity] {
		granularity = defaultGranularity
	}

	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		loc = time.UTC
	}

	from, to := parseDateRange(r)

	// The previous period is the same window shifted back by its own length. Scanning from
	// its start means one round trip covers both windows, and each is then folded with the
	// same inRange the rest of the codebase uses.
	previousFrom := dashboardPreviousFrom(from, to)
	scanFrom := from
	if previousFrom != nil {
		scanFrom = previousFrom
	}

	query := h.DB.Table("issuances").
		Select(`issuances.points,
			issuances.value_of_the_thousand,
			issuances.taxes,
			issuances.refunded_points,
			issuances.created_at,
			issuances.cancelled_at,
			issuances.loyalty_program_id,
			loyalty_programs.name AS loyalty_program_name,
			loyalty_programs.color AS loyalty_program_color,
			issuances.issuer_id,
			users.name AS issuer_name`).
		Joins("JOIN loyalty_programs ON loyalty_programs.id = issuances.loyalty_program_id").
		Joins("JOIN users ON users.id = issuances.issuer_id").
		// A deleted issuance is a typo being erased - it never happened, so it contributes
		// to nothing. The join is hand-written, so GORM does not apply this scope for us.
		Where("issuances.deleted_at IS NULL").
		Where("issuances.agency_id = ?", agencyID)

	q := r.URL.Query()
	for column, param := range map[string]string{
		"issuances.loyalty_program_id": "loyalty_program_id",
		"issuances.airline_id":         "airline_id",
		"issuances.issuer_id":          "issuer_id",
	} {
		if value := q.Get(param); value != "" {
			query = query.Where(column+" = ?", value)
		}
	}

	// Narrow to rows that can contribute at all: created in the window, or cancelled in it.
	// Without this the aggregation would read the whole table.
	if scanFrom != nil || to != nil {
		created := h.DB.Session(&gorm.Session{NewDB: true})
		cancelled := h.DB.Session(&gorm.Session{NewDB: true})
		if scanFrom != nil {
			created = created.Where("issuances.created_at >= ?", *scanFrom)
			cancelled = cancelled.Where("issuances.cancelled_at >= ?", *scanFrom)
		}
		if to != nil {
			created = created.Where("issuances.created_at < ?", *to)
			cancelled = cancelled.Where("issuances.cancelled_at < ?", *to)
		}
		query = query.Where(created.Or(cancelled))
	}

	var rows []dashboardIssuanceRow
	if err := query.Scan(&rows).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch dashboard metrics")
		return
	}

	resp := aggregateDashboard(rows, from, to, granularity, loc, includeAmount)
	resp.Range = schemas.DashboardRange{
		From:        formatDashboardBound(from, loc, false),
		To:          formatDashboardBound(to, loc, true),
		Granularity: granularity,
	}

	utils.SendJSON(w, r, http.StatusOK, "Dashboard metrics fetched successfully", resp)
}

// dashboardPreviousFrom is the start of the window immediately before [from, to). Nil when
// either bound is missing - "the previous period" has no meaning without a length.
func dashboardPreviousFrom(from, to *time.Time) *time.Time {
	if from == nil || to == nil {
		return nil
	}
	previous := from.Add(-to.Sub(*from))
	return &previous
}

// formatDashboardBound renders a UTC bound back as the local calendar day the caller asked
// for. The upper bound is exclusive (parseDateRange advanced it to the next day), so it is
// stepped back a day to echo the inclusive date the request actually used.
func formatDashboardBound(bound *time.Time, loc *time.Location, exclusive bool) string {
	if bound == nil {
		return ""
	}
	local := bound.In(loc)
	if exclusive {
		local = local.AddDate(0, 0, -1)
	}
	return local.Format("2006-01-02")
}

// aggregateDashboard folds per-issuance rows into every panel of the dashboard at once, so
// the tiles and the charts are guaranteed to be reading the same numbers.
//
// A cancelled issuance is excluded from volume entirely - count, points, amount, series,
// by-program and by-issuer alike, in the current window and the previous one. Dropping it
// from the count as well as from the money is what keeps "average per issuance" honest:
// numerator and denominator have to move together.
//
// Cancellations are still reported, by when they were cancelled, through CancelledCount and
// RefundedPoints. Those two are now the only place a cancellation shows up on the dashboard.
//
// The volume rollups (series, by-program, by-issuer) follow creation - a cancellation is not
// something an issuer "emitted", so folding it into their ranking would misread the chart's
// question.
func aggregateDashboard(
	rows []dashboardIssuanceRow,
	from, to *time.Time,
	granularity string,
	loc *time.Location,
	includeAmount bool,
) schemas.DashboardResponse {
	previousFrom := dashboardPreviousFrom(from, to)

	buckets := map[string]*dashboardAccumulator{}
	byProgram := map[string]*dashboardAccumulator{}
	byIssuer := map[string]*dashboardAccumulator{}

	// Seeded to zero rather than left nil so the field is always present for an admin, even
	// for a window with no issuances at all. Absent has to mean exactly one thing - "you are
	// not allowed to see this" - or the frontend cannot tell a withheld figure from a quiet
	// period, and would drop the delta on the stat tiles whenever the previous period was
	// empty. stripDashboardAmounts puts the nils back for non-admins.
	totals := schemas.DashboardTotals{AmountCents: addAmount(nil, 0)}
	previous := schemas.DashboardTotals{AmountCents: addAmount(nil, 0)}

	add := func(into map[string]*dashboardAccumulator, key, name, color string, row dashboardIssuanceRow) {
		acc, ok := into[key]
		if !ok {
			acc = &dashboardAccumulator{name: name, color: color}
			into[key] = acc
		}
		acc.points += row.Points
		acc.amountCents += row.amountCents()
		acc.issuanceCount++
	}

	for _, row := range rows {
		// A cancelled issuance produced nothing, so it contributes nothing to volume - not a
		// count, not a mile, not a cent, in any window. It stays in this loop only to feed the
		// cancellation tallies below.
		//
		// The test is "is it cancelled at all", not "was it cancelled inside this window": an
		// issuance created in January and cancelled in March disappears from January too. That
		// makes a closed month's figure move when somebody cancels today, which is a deliberate
		// trade for the totals always describing what actually stands.
		if row.CancelledAt == nil {
			createdAt := row.CreatedAt

			if inRange(&createdAt, from, to) {
				totals.IssuanceCount++
				totals.Points += row.Points
				totals.AmountCents = addAmount(totals.AmountCents, row.amountCents())

				add(buckets, dashboardBucketKey(createdAt, granularity, loc), "", "", row)
				add(byProgram, row.LoyaltyProgramID, row.LoyaltyProgramName, row.LoyaltyProgramColor, row)
				add(byIssuer, row.IssuerID, row.IssuerName, "", row)
			} else if inRange(&createdAt, previousFrom, from) {
				previous.IssuanceCount++
				previous.Points += row.Points
				previous.AmountCents = addAmount(previous.AmountCents, row.amountCents())
			}
		}

		if inRange(row.CancelledAt, from, to) {
			totals.CancelledCount++
			if row.RefundedPoints != nil {
				totals.RefundedPoints += *row.RefundedPoints
			}
		} else if inRange(row.CancelledAt, previousFrom, from) {
			previous.CancelledCount++
			if row.RefundedPoints != nil {
				previous.RefundedPoints += *row.RefundedPoints
			}
		}
	}

	resp := schemas.DashboardResponse{
		Totals:    totals,
		Series:    make([]schemas.DashboardBucket, 0, len(buckets)),
		ByProgram: make([]schemas.DashboardBreakdown, 0, len(byProgram)),
		ByIssuer:  make([]schemas.DashboardBreakdown, 0, len(byIssuer)),
	}

	// Every bucket in the window is emitted, not just the ones with data, so the line does
	// not jump straight across a quiet Sunday as if it were a single long day.
	for _, key := range dashboardBucketKeys(rows, from, to, granularity, loc) {
		bucket := schemas.DashboardBucket{Bucket: key}
		if acc, ok := buckets[key]; ok {
			bucket.IssuanceCount = acc.issuanceCount
			bucket.Points = acc.points
			bucket.AmountCents = addAmount(nil, acc.amountCents)
		} else {
			bucket.AmountCents = addAmount(nil, 0)
		}
		resp.Series = append(resp.Series, bucket)
	}

	resp.ByProgram = breakdownFrom(byProgram)
	resp.ByIssuer = breakdownFrom(byIssuer)

	if previousFrom != nil {
		resp.PreviousTotals = &previous
	}

	if !includeAmount {
		stripDashboardAmounts(&resp)
	}

	return resp
}

// addAmount accumulates into an optional money field. Kept as a helper because the totals
// carry *int - the pointer is what makes the field disappear for non-admins - and summing
// through a pointer inline at four call sites reads badly.
func addAmount(current *int, delta int) *int {
	if current == nil {
		value := delta
		return &value
	}
	*current += delta
	return current
}

// stripDashboardAmounts nils every money field. Called once, at the end, so the aggregation
// itself never has to thread the permission through - a rollup that forgot the flag would
// leak, whereas a single sweep over the finished payload cannot miss a field.
func stripDashboardAmounts(resp *schemas.DashboardResponse) {
	resp.Totals.AmountCents = nil
	if resp.PreviousTotals != nil {
		resp.PreviousTotals.AmountCents = nil
	}
	for i := range resp.Series {
		resp.Series[i].AmountCents = nil
	}
	for i := range resp.ByProgram {
		resp.ByProgram[i].AmountCents = nil
	}
	for i := range resp.ByIssuer {
		resp.ByIssuer[i].AmountCents = nil
	}
}

// dashboardBucketStart snaps an instant down to the start of its bucket, in local time.
// Weeks start on Monday, matching how the agency reports its numbers (and the frontend's
// resolvePeriodRange).
func dashboardBucketStart(t time.Time, granularity string, loc *time.Location) time.Time {
	local := t.In(loc)
	switch granularity {
	case "hour":
		return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, loc)
	case "week":
		// Weekday() is 0 for Sunday, so Sunday has to walk back six days, not zero.
		weekday := int(local.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		return day.AddDate(0, 0, -(weekday - 1))
	case "month":
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	default:
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	}
}

// dashboardBucketNext steps one bucket forward. AddDate rather than a fixed Duration for
// week and month, so the step follows the calendar instead of assuming equal-length months.
func dashboardBucketNext(t time.Time, granularity string) time.Time {
	switch granularity {
	case "hour":
		return t.Add(time.Hour)
	case "week":
		return t.AddDate(0, 0, 7)
	case "month":
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}

func formatDashboardBucket(start time.Time, granularity string) string {
	switch granularity {
	case "hour":
		return start.Format("2006-01-02T15:00")
	case "month":
		return start.Format("2006-01")
	default:
		return start.Format("2006-01-02")
	}
}

func dashboardBucketKey(t time.Time, granularity string, loc *time.Location) string {
	return formatDashboardBucket(dashboardBucketStart(t, granularity, loc), granularity)
}

// dashboardBucketKeys lists every bucket the series should carry, in order. It walks the
// requested window when there is one; with an open-ended request it falls back to the span
// the data itself covers, so an unfiltered call still returns a sane axis instead of
// nothing.
func dashboardBucketKeys(
	rows []dashboardIssuanceRow,
	from, to *time.Time,
	granularity string,
	loc *time.Location,
) []string {
	first, last, ok := dashboardSeriesSpan(rows, from, to)
	if !ok {
		return nil
	}

	keys := []string{}
	// `last` is an instant inside the final bucket, so the walk continues while the cursor
	// has not passed it - not while it is below an exclusive upper bound.
	for cursor := dashboardBucketStart(first, granularity, loc); !cursor.After(last); cursor = dashboardBucketNext(cursor, granularity) {
		keys = append(keys, formatDashboardBucket(cursor, granularity))
	}
	return keys
}

// dashboardSeriesSpan resolves the instants the axis has to cover. The requested bounds win
// when present; otherwise the earliest and latest creation among the rows stand in.
func dashboardSeriesSpan(rows []dashboardIssuanceRow, from, to *time.Time) (first, last time.Time, ok bool) {
	if from != nil && to != nil {
		// `to` is exclusive; step back inside the window so the walk does not emit a
		// trailing empty bucket for the day after the range.
		return *from, to.Add(-time.Nanosecond), true
	}

	for _, row := range rows {
		if !inRange(&row.CreatedAt, from, to) {
			continue
		}
		if !ok || row.CreatedAt.Before(first) {
			first = row.CreatedAt
		}
		if !ok || row.CreatedAt.After(last) {
			last = row.CreatedAt
		}
		ok = true
	}
	return first, last, ok
}

// dashboardAccumulator is the running total behind one key of a rollup - a time bucket, a
// loyalty program, or an issuer. All three fold identically, so they share the shape.
type dashboardAccumulator struct {
	name          string
	color         string
	points        int
	amountCents   int
	issuanceCount int
}

// breakdownFrom turns an accumulator map into the sorted rollup the charts render. Biggest
// first, name breaking ties so the order - and therefore each entity's colour slot on the
// frontend - is stable across requests (ranging over a map is deliberately randomised).
func breakdownFrom(accumulators map[string]*dashboardAccumulator) []schemas.DashboardBreakdown {
	out := make([]schemas.DashboardBreakdown, 0, len(accumulators))
	for id, acc := range accumulators {
		out = append(out, schemas.DashboardBreakdown{
			ID:            id,
			Name:          acc.name,
			Color:         acc.color,
			Points:        acc.points,
			AmountCents:   addAmount(nil, acc.amountCents),
			IssuanceCount: acc.issuanceCount,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Points != out[j].Points {
			return out[i].Points > out[j].Points
		}
		return out[i].Name < out[j].Name
	})

	return out
}
