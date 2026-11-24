package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/murandido/m-80-backend/internal/businessday"
	"github.com/murandido/m-80-backend/internal/data"
)

// FreeWriteTools are the tools that write without asking when the token allows it. A token's
// allowed_write_tools is validated against this list.
var FreeWriteTools = []string{
	"create_issuance",
	"update_issuance",
	"add_adjustment",
	"update_adjustment",
	"sync_issuance_traveos",
	"add_point_batch",
	"update_point_batch",
	"record_statement_batches",
	"import_spreadsheet_issuances",
}

// batchCreatingTools are the free writes that add points to the stock, the ones
// max_batches_per_day counts.
var batchCreatingTools = []string{"add_point_batch", "record_statement_batches"}

// tokenPolicy is what a token with a write policy may do on its own. A token without one has
// no tokenPolicy at all (caller.Policy == nil).
type tokenPolicy struct {
	// AllowedWriteTools nil would mean "all", but a policy only exists when the column is set,
	// so here it is always the explicit list - possibly empty.
	AllowedWriteTools []string
	Limits            data.MCPTokenLimits
}

// policyOf reads the token's policy. Both columns NULL is the token as it was before policies
// existed: nil, everything direct. Limits without a tool list still restrict nothing by tool.
func policyOf(token data.MCPToken) *tokenPolicy {
	if token.AllowedWriteTools == nil && token.Limits == nil {
		return nil
	}
	policy := &tokenPolicy{AllowedWriteTools: FreeWriteTools}
	if token.AllowedWriteTools != nil {
		policy.AllowedWriteTools = []string(token.AllowedWriteTools)
	}
	if token.Limits != nil {
		policy.Limits = *token.Limits
	}
	return policy
}

// writeIntent is a free write described before it happens, so it can either run or become a
// proposal for the same operation.
type writeIntent struct {
	Operation Operation
	Summary   string
	// Preview is fetched only when the write becomes a proposal: running it directly would
	// pay for a call whose answer nobody reads.
	Preview func() (any, error)
	// Reasons are why this write needs a human even if the tool is allowed - a ceiling
	// crossed, a duplicate, a credit the statement does not show. Empty means none.
	Reasons []string
}

// callOutcome is how a tool body tells addTool that a write became a proposal, so the call is
// recorded as such and not counted as a write against the token's daily ceiling.
type callOutcome struct{ Proposed bool }

type outcomeKey struct{}

func outcomeFrom(ctx context.Context) *callOutcome {
	outcome, _ := ctx.Value(outcomeKey{}).(*callOutcome)
	return outcome
}

// writeOrPropose runs a free write directly when the token may, and records it as a proposal
// otherwise. A token with no policy always runs it - the behaviour every token had before.
func (s *Server) writeOrPropose(ctx context.Context, who caller, tool string, intent writeIntent) (any, error) {
	reasons := intent.Reasons
	if who.Policy != nil && !slices.Contains(who.Policy.AllowedWriteTools, tool) {
		reasons = append([]string{"ferramenta não liberada para escrever sozinha neste token"}, reasons...)
	}
	if who.Policy == nil {
		// No policy, no ceilings: the reasons only exist to protect an unattended agent.
		reasons = nil
	}

	if len(reasons) == 0 {
		var body any
		if len(intent.Operation.Body) > 0 {
			body = intent.Operation.Body
		}
		return s.call(ctx, who, intent.Operation.Method, intent.Operation.Path, nil, body)
	}

	var preview any
	if intent.Preview != nil {
		p, err := intent.Preview()
		if err != nil {
			return nil, err
		}
		preview = p
	}
	if outcome := outcomeFrom(ctx); outcome != nil {
		outcome.Proposed = true
	}
	summary := fmt.Sprintf("%s [virou proposta: %s]", intent.Summary, strings.Join(reasons, "; "))
	return s.propose(who, tool, summary, intent.Operation, preview)
}

// batchesWrittenToday counts the batch-creating calls this token ran directly today (São
// Paulo day). Calls that became proposals are marked in their args and left out.
func (s *Server) batchesWrittenToday(who caller) (int64, error) {
	start := businessday.Start(businessday.Of(s.now()))
	var count int64
	err := s.db.Model(&data.MCPCall{}).
		Where("token_id = ? AND ok AND tool IN ? AND created_at >= ?", who.TokenID, batchCreatingTools, start).
		Where("NOT (COALESCE(args, '{}'::jsonb) ? '_outcome')").
		Count(&count).Error
	return count, err
}

// batchReasons are the ceilings that writing batches would cross for this token: points is
// one batch's points (0 to skip that check), batches is how many new batches the write adds
// (0 to skip the daily count, as a correction adds none).
func (s *Server) batchReasons(who caller, points, batches int) ([]string, error) {
	var reasons []string
	if reason := pointsReason(who, points); reason != "" {
		reasons = append(reasons, reason)
	}
	reason, err := s.dailyReason(who, batches)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		reasons = append(reasons, reason)
	}
	return reasons, nil
}

func pointsReason(who caller, points int) string {
	if who.Policy == nil || who.Policy.Limits.MaxBatchPoints == nil || points <= *who.Policy.Limits.MaxBatchPoints {
		return ""
	}
	return fmt.Sprintf("%d pontos acima do limite de %d por lote", points, *who.Policy.Limits.MaxBatchPoints)
}

func (s *Server) dailyReason(who caller, batches int) (string, error) {
	if who.Policy == nil || who.Policy.Limits.MaxBatchesPerDay == nil || batches == 0 {
		return "", nil
	}
	done, err := s.batchesWrittenToday(who)
	if err != nil {
		return "", err
	}
	if int(done)+batches <= *who.Policy.Limits.MaxBatchesPerDay {
		return "", nil
	}
	return fmt.Sprintf("limite de %d lotes por dia atingido (%d já lançados hoje)", *who.Policy.Limits.MaxBatchesPerDay, done), nil
}

// batchSource is where an agent read the batch it is recording. Required from a token with a
// policy: a batch whose origin nobody can trace back is the one nobody can check either.
type batchSource struct {
	Channel   string `json:"channel" jsonschema:"De onde veio a informação: whatsapp ou statement."`
	Chat      string `json:"chat,omitempty" jsonschema:"Nome do chat do WhatsApp, quando channel=whatsapp."`
	MessageAt string `json:"message_at,omitempty" jsonschema:"Data e hora da mensagem (AAAA-MM-DD HH:MM)."`
	Excerpt   string `json:"excerpt,omitempty" jsonschema:"Trecho curto da mensagem que justifica o lote (até 200 caracteres)."`
}

// note is the batch note an agent's batch carries, so the panel shows where it came from
// without a column of its own.
func (src batchSource) note(extra *string) string {
	excerpt := strings.TrimSpace(src.Excerpt)
	if len([]rune(excerpt)) > 200 {
		excerpt = string([]rune(excerpt)[:200]) + "…"
	}
	parts := []string{"[Agente]", src.Channel}
	if src.Chat != "" {
		parts = append(parts, src.Chat)
	}
	if src.MessageAt != "" {
		parts = append(parts, src.MessageAt)
	}
	note := strings.Join(parts, " ")
	if excerpt != "" {
		note += ": " + excerpt
	}
	if extra != nil && strings.TrimSpace(*extra) != "" {
		note += " | " + strings.TrimSpace(*extra)
	}
	return note
}

func requireSource(who caller, src *batchSource) error {
	if who.Policy == nil {
		return nil
	}
	if src == nil || (src.Channel != "whatsapp" && src.Channel != "statement") {
		return fmt.Errorf("informe source (channel whatsapp ou statement, com chat, horário e trecho): este token precisa registrar de onde veio cada lote")
	}
	return nil
}

// statementCredit is one credit of the statement reconciliation.
type statementCredit struct {
	Index       int    `json:"index"`
	OccurredOn  string `json:"occurred_on"`
	Points      int    `json:"points"`
	Description string `json:"description"`
	Verdict     string `json:"verdict"`
	BatchID     string `json:"batch_id,omitempty"`
}

// reconciliationPath is the statement reconciliation route for the account's programme, or ""
// when the programme has no statement.
func reconciliationPath(accountID, programCode string) string {
	switch programCode {
	case "AZUL":
		return "/accounts/" + url.PathEscape(accountID) + "/azul-statement"
	case "SMILES":
		return "/accounts/" + url.PathEscape(accountID) + "/smiles-statement"
	}
	return ""
}

type reconciliationEnvelope struct {
	SyncState      json.RawMessage `json:"sync_state"`
	Reconciliation struct {
		From              string            `json:"from"`
		To                string            `json:"to"`
		Bookings          []json.RawMessage `json:"bookings"`
		Refunds           []json.RawMessage `json:"refunds"`
		Credits           []statementCredit `json:"credits"`
		StatementNetPoint int               `json:"statement_net_points"`
		PanelNetPoints    int               `json:"panel_net_points"`
	} `json:"reconciliation"`
}

// loadReconciliation reads the statement reconciliation of an account, choosing the
// programme's route from the account itself.
func (s *Server) loadReconciliation(ctx context.Context, who caller, accountID string) (string, reconciliationEnvelope, error) {
	var account apiAccount
	if err := s.get(ctx, who, "/accounts/"+url.PathEscape(accountID), nil, &account); err != nil {
		return "", reconciliationEnvelope{}, err
	}
	path := reconciliationPath(accountID, account.LoyaltyProgram.Code)
	if path == "" {
		return account.LoyaltyProgram.Code, reconciliationEnvelope{}, fmt.Errorf("a conta é do programa %s, que não tem extrato para conciliar", account.LoyaltyProgram.Code)
	}
	var env reconciliationEnvelope
	if err := s.get(ctx, who, path, nil, &env); err != nil {
		return account.LoyaltyProgram.Code, env, err
	}
	return account.LoyaltyProgram.Code, env, nil
}

// statementMatch reports whether the statement shows a credit still missing in the panel with
// exactly this day and points - the only kind of batch an agent with a policy may record by
// itself, because then the statement vouches for the points and the agent supplies only the
// cost and who paid.
func (s *Server) statementMatch(ctx context.Context, who caller, accountID, acquiredOn string, points int) (bool, error) {
	_, env, err := s.loadReconciliation(ctx, who, accountID)
	if err != nil {
		return false, err
	}
	for _, credit := range env.Reconciliation.Credits {
		if credit.Verdict == "missing_in_panel" && credit.OccurredOn == acquiredOn && credit.Points == points {
			return true, nil
		}
	}
	return false, nil
}

// duplicateBatch reports whether the account already has a batch on that day with those points.
func (s *Server) duplicateBatch(ctx context.Context, who caller, accountID, acquiredOn string, points int) (bool, error) {
	var batches []struct {
		AcquiredOn string `json:"acquired_on"`
		Points     int    `json:"points"`
	}
	query := url.Values{"account_id": {accountID}, "from": {acquiredOn}, "to": {acquiredOn}}
	if err := s.get(ctx, who, "/point-batches", query, &batches); err != nil {
		return false, err
	}
	for _, batch := range batches {
		if batch.AcquiredOn == acquiredOn && batch.Points == points {
			return true, nil
		}
	}
	return false, nil
}
