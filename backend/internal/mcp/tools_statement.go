package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// statementSummary is the reconciliation with the matched lines left out: what the agent has
// to look at, and nothing it would only scroll past.
type statementSummary struct {
	Program            string            `json:"program"`
	SyncState          json.RawMessage   `json:"sync_state,omitempty"`
	From               string            `json:"from"`
	To                 string            `json:"to"`
	StatementNetPoints int               `json:"statement_net_points"`
	PanelNetPoints     int               `json:"panel_net_points"`
	CreditsWithoutLink []statementCredit `json:"credits_needing_attention"`
	Bookings           []json.RawMessage `json:"bookings_needing_attention"`
	Refunds            []json.RawMessage `json:"refunds_needing_attention"`
	// The totals before truncate cut the lists, so a capped list still says how much is out
	// there. Equal to the list lengths when nothing was cut.
	CreditsTotal  int  `json:"credits_needing_attention_total"`
	BookingsTotal int  `json:"bookings_needing_attention_total"`
	RefundsTotal  int  `json:"refunds_needing_attention_total"`
	Truncated     bool `json:"truncated,omitempty"`
}

// truncate keeps at most max items per list. The daily check runs every day for every
// injunction account, and a statement nobody has reconciled yet can carry hundreds of lines -
// tokens the routine pays for without reading. statement_reconciliation still has them all.
func (summary *statementSummary) truncate(limit int) {
	if len(summary.CreditsWithoutLink) > limit {
		summary.CreditsWithoutLink = summary.CreditsWithoutLink[:limit]
		summary.Truncated = true
	}
	if len(summary.Bookings) > limit {
		summary.Bookings = summary.Bookings[:limit]
		summary.Truncated = true
	}
	if len(summary.Refunds) > limit {
		summary.Refunds = summary.Refunds[:limit]
		summary.Truncated = true
	}
}

func unmatched(items []json.RawMessage) []json.RawMessage {
	out := []json.RawMessage{}
	for _, item := range items {
		var line struct {
			Verdict string `json:"verdict"`
		}
		if json.Unmarshal(item, &line) == nil && line.Verdict == "matched" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (s *Server) statementSummary(ctx context.Context, who caller, accountID string) (statementSummary, error) {
	program, env, err := s.loadReconciliation(ctx, who, accountID)
	if err != nil {
		return statementSummary{}, err
	}
	summary := statementSummary{
		Program:            program,
		SyncState:          env.SyncState,
		From:               env.Reconciliation.From,
		To:                 env.Reconciliation.To,
		StatementNetPoints: env.Reconciliation.StatementNetPoint,
		PanelNetPoints:     env.Reconciliation.PanelNetPoints,
		CreditsWithoutLink: []statementCredit{},
		Bookings:           unmatched(env.Reconciliation.Bookings),
		Refunds:            unmatched(env.Reconciliation.Refunds),
	}
	for _, credit := range env.Reconciliation.Credits {
		if credit.Verdict != "matched" {
			summary.CreditsWithoutLink = append(summary.CreditsWithoutLink, credit)
		}
	}
	summary.CreditsTotal = len(summary.CreditsWithoutLink)
	summary.BookingsTotal = len(summary.Bookings)
	summary.RefundsTotal = len(summary.Refunds)
	return summary, nil
}

type statementBatchChoice struct {
	CreditIndex     int          `json:"credit_index" jsonschema:"O index do crédito em statement_reconciliation (credits_needing_attention[].index), com verdict missing_in_panel."`
	OriginID        string       `json:"origin_id" jsonschema:"ID da origem (compra, transferência, bônus...). Veja list_point_batch_origins."`
	CostCents       int          `json:"cost_cents" jsonschema:"Quanto custou este crédito, em centavos. Zero para bônus/acúmulo."`
	FundingAgencyID string       `json:"funding_agency_id" jsonschema:"ID da agência que pagou. Veja list_agencies."`
	Note            *string      `json:"note,omitempty" jsonschema:"Observação livre."`
	Source          *batchSource `json:"source,omitempty" jsonschema:"De onde veio o custo/agência (obrigatório para tokens de agente)."`
}

type recordStatementBatchesInput struct {
	AccountID string                 `json:"account_id" jsonschema:"ID da conta (Azul ou Smiles, com estoque próprio e extrato)."`
	Batches   []statementBatchChoice `json:"batches" jsonschema:"Um item por crédito do extrato a virar lote."`
}

func (s *Server) registerStatementTools(server *sdk.Server) {
	addTool(s, server, &sdk.Tool{
		Name: "statement_reconciliation",
		Description: "Conciliação do extrato Azul/Smiles de uma conta com o painel, só com o que não bate: créditos sem lote (missing_in_panel), " +
			"emissões e estornos que faltam no painel ou no extrato, e diferenças de valor. Pontos líquidos do extrato e do painel no período lido. " +
			"Lê o extrato já sincronizado; não consulta a cia.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in accountIDInput) (any, error) {
		return s.statementSummary(ctx, who, in.AccountID)
	})

	addTool(s, server, &sdk.Tool{
		Name: "record_statement_batches",
		Description: "Lança lotes a partir de créditos do extrato ainda sem lote: os pontos e a data vêm do extrato; você informa origem, custo (centavos) e agência pagadora. " +
			"Créditos que já ganharam lote são ignorados, e o lote fica vinculado ao extrato. É o jeito preferido de lançar lote. " +
			"Token de agente: exige source em cada item e respeita os limites do token; o que passar deles vira proposta.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, func(ctx context.Context, who caller, in recordStatementBatchesInput) (any, error) {
		if len(in.Batches) == 0 {
			return nil, fmt.Errorf("informe ao menos um crédito em batches")
		}
		program, env, err := s.loadReconciliation(ctx, who, in.AccountID)
		if err != nil {
			return nil, err
		}

		credits := map[int]statementCredit{}
		for _, credit := range env.Reconciliation.Credits {
			credits[credit.Index] = credit
		}

		type selection struct {
			Index           int    `json:"index"`
			OriginID        string `json:"origin_id"`
			CostCents       int    `json:"cost_cents"`
			FundingAgencyID string `json:"funding_agency_id"`
			Note            string `json:"note"`
		}
		selections := make([]selection, 0, len(in.Batches))
		chosen := make([]statementCredit, 0, len(in.Batches))
		var reasons []string
		for _, choice := range in.Batches {
			if err := requireSource(who, choice.Source); err != nil {
				return nil, err
			}
			credit, ok := credits[choice.CreditIndex]
			if !ok || credit.Verdict != "missing_in_panel" {
				return nil, fmt.Errorf("o crédito %d não está sem lote no extrato; confira statement_reconciliation", choice.CreditIndex)
			}
			note := ""
			if choice.Note != nil {
				note = *choice.Note
			}
			if choice.Source != nil {
				// The source makes every note distinct, so each credit becomes its own batch
				// instead of being summed with others of the same origin and payer: an agent's
				// batch has to be traceable to the one message that priced it.
				note = choice.Source.note(choice.Note)
			}
			selections = append(selections, selection{
				Index: credit.Index, OriginID: choice.OriginID, CostCents: choice.CostCents,
				FundingAgencyID: choice.FundingAgencyID, Note: note,
			})
			chosen = append(chosen, credit)

			if reason := pointsReason(who, credit.Points); reason != "" {
				reasons = append(reasons, reason)
			}
		}
		if reason, err := s.dailyReason(who, len(selections)); err != nil {
			return nil, err
		} else if reason != "" {
			reasons = append(reasons, reason)
		}

		total := 0
		for _, credit := range chosen {
			total += credit.Points
		}
		return s.writeOrPropose(ctx, who, "record_statement_batches", writeIntent{
			Operation: Operation{
				Method: http.MethodPost,
				Path:   reconciliationPath(in.AccountID, program) + "/commit",
				Body:   bodyOf(map[string]any{"batches": selections}),
			},
			Summary: fmt.Sprintf("Lançar %d lote(s) do extrato %s, %d pontos no total", len(selections), program, total),
			Preview: func() (any, error) { return map[string]any{"credits": chosen, "choices": in.Batches}, nil },
			Reasons: reasons,
		})
	})
}
