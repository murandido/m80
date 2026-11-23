package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/businessday"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
)

// categoryInjunction is accounts.category for an account under a court order ("liminar").
const categoryInjunction = "injunction"

type dailyCheckInput struct {
	Date     string `json:"date,omitempty" jsonschema:"Dia da conferência (AAAA-MM-DD). Vazio = hoje. O fechamento vai do dia 1 do mês até esta data."`
	MaxItems int    `json:"max_items,omitempty" jsonschema:"Máximo de linhas do extrato por lista e por conta (padrão 30); os totais vêm sempre. Para ver tudo de uma conta, use statement_reconciliation."`
}

const defaultDailyCheckItems = 30

type unsettledBatch struct {
	BatchID           string  `json:"batch_id"`
	AcquiredOn        string  `json:"acquired_on"`
	Points            int     `json:"points"`
	CostCents         int     `json:"cost_cents"`
	RemainingPoints   int     `json:"remaining_points"`
	FundingAgencyName *string `json:"funding_agency_name"`
}

type closingAccount struct {
	AccountID     string `json:"account_id"`
	PendingPoints int    `json:"pending_points"`
	Closed        bool   `json:"closed"`
	Batches       []struct {
		unsettledBatch
		SettledAt *string `json:"settled_at"`
	} `json:"batches"`
	Settlements json.RawMessage `json:"settlements"`
	Parts       json.RawMessage `json:"parts"`
}

type closingCheck struct {
	PendingPoints    int              `json:"pending_points"`
	PeriodClosed     bool             `json:"period_closed"`
	UnsettledBatches []unsettledBatch `json:"unsettled_batches"`
	Settlements      json.RawMessage  `json:"settlements"`
	Parts            json.RawMessage  `json:"parts"`
}

type agentBatch struct {
	ID         string `json:"id"`
	AcquiredOn string `json:"acquired_on"`
	Points     int    `json:"points"`
	CostCents  int    `json:"cost_cents"`
	Note       string `json:"note"`
	CreatedAt  string `json:"created_at"`
}

type accountCheck struct {
	stockAccount
	Alerts         []stockAlert      `json:"alerts"`
	Statement      *statementSummary `json:"statement,omitempty"`
	StatementError string            `json:"statement_error,omitempty"`
	Closing        *closingCheck     `json:"closing,omitempty"`
	ClosingError   string            `json:"closing_error,omitempty"`
	AgentBatches   []agentBatch      `json:"agent_batches_since_last_report"`
}

type saveReportInput struct {
	ReportDate string          `json:"report_date,omitempty" jsonschema:"Dia a que o relatório se refere (AAAA-MM-DD). Vazio = hoje. Salvar de novo no mesmo dia substitui o relatório."`
	Title      string          `json:"title" jsonschema:"Título curto, ex.: Conferência das liminares - 23/11."`
	Summary    string          `json:"summary" jsonschema:"Resumo em texto (pode usar quebras de linha): o que foi conferido, o que bate e o que não bate."`
	Findings   []reportFinding `json:"findings,omitempty" jsonschema:"Um item por problema encontrado."`
	Actions    []reportAction  `json:"actions,omitempty" jsonschema:"O que você fez: lotes lançados, propostas criadas."`
}

type reportFinding struct {
	AccountID string `json:"account_id,omitempty" jsonschema:"Conta do achado, quando houver."`
	Kind      string `json:"kind" jsonschema:"Tipo curto, ex.: balance_divergence, credit_without_batch, booking_missing, pending_points, unsettled_batch."`
	Severity  string `json:"severity" jsonschema:"info, warning ou critical."`
	Detail    string `json:"detail" jsonschema:"Explicação em uma ou duas frases, com os números."`
}

type reportAction struct {
	Tool        string `json:"tool" jsonschema:"Ferramenta usada, ex.: record_statement_batches, propose_cancel_issuance."`
	Description string `json:"description" jsonschema:"O que foi feito."`
	ProposalID  string `json:"proposal_id,omitempty" jsonschema:"ID da proposta criada, quando virou proposta."`
}

func (s *Server) lastReportAt(who caller) (time.Time, error) {
	var report data.MCPReport
	err := s.db.Where("token_id = ?", who.TokenID).Order("updated_at desc").Limit(1).Find(&report).Error
	return report.UpdatedAt, err
}

func (s *Server) agentBatchesSince(ctx context.Context, who caller, accountID string, since time.Time) ([]agentBatch, error) {
	var batches []agentBatch
	if err := s.get(ctx, who, "/point-batches", url.Values{"account_id": {accountID}}, &batches); err != nil {
		return nil, err
	}
	out := []agentBatch{}
	for _, batch := range batches {
		if !strings.HasPrefix(batch.Note, "[Agente]") {
			continue
		}
		created, err := time.Parse(time.RFC3339Nano, batch.CreatedAt)
		if err == nil && !created.After(since) {
			continue
		}
		out = append(out, batch)
	}
	return out, nil
}

func (s *Server) closingChecks(ctx context.Context, who caller, accountIDs []string, day time.Time) (map[string]*closingCheck, error) {
	out := map[string]*closingCheck{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	from := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	var closing struct {
		Accounts []closingAccount `json:"accounts"`
	}
	query := url.Values{
		"account_ids": {strings.Join(accountIDs, ",")},
		"from":        {from.Format(time.DateOnly)},
		"to":          {day.Format(time.DateOnly)},
	}
	if err := s.get(ctx, who, "/inventory-closing", query, &closing); err != nil {
		return nil, err
	}
	for _, account := range closing.Accounts {
		check := &closingCheck{
			PendingPoints:    account.PendingPoints,
			PeriodClosed:     account.Closed,
			UnsettledBatches: []unsettledBatch{},
			Settlements:      account.Settlements,
			Parts:            account.Parts,
		}
		for _, batch := range account.Batches {
			if batch.SettledAt == nil {
				check.UnsettledBatches = append(check.UnsettledBatches, batch.unsettledBatch)
			}
		}
		out[account.AccountID] = check
	}
	return out, nil
}

func (s *Server) reportDay(raw string) (time.Time, error) {
	if raw == "" {
		return businessday.Of(s.now()), nil
	}
	day, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("data inválida %q: use AAAA-MM-DD", raw)
	}
	return day, nil
}

func (s *Server) registerReportTools(server *sdk.Server) {
	addTool(s, server, &sdk.Tool{
		Name: "injunction_daily_check",
		Description: "Conferência das contas liminares (categoria injunction) numa chamada só. Para cada conta: saldo esperado × real, divergência, " +
			"sincronização e alertas; conciliação do extrato (créditos sem lote, emissões e estornos que não batem); fechamento do mês até a data " +
			"(pontos sem lote, lotes não acertados entre agências, acertos e divisão de lucro, em centavos); e os lotes lançados pelo agente desde o último relatório. " +
			"Uma parte que falhar vem como *_error, sem derrubar o resto.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in dailyCheckInput) (any, error) {
		day, err := s.reportDay(in.Date)
		if err != nil {
			return nil, err
		}
		accounts, err := s.listStockAccounts(ctx, who, "", categoryInjunction)
		if err != nil {
			return nil, err
		}
		since, err := s.lastReportAt(who)
		if err != nil {
			return nil, err
		}

		var inventoryIDs []string
		for _, account := range accounts {
			if account.OwnsPointInventory {
				inventoryIDs = append(inventoryIDs, account.ID)
			}
		}
		closings, closingErr := s.closingChecks(ctx, who, inventoryIDs, day)

		checks := make([]accountCheck, 0, len(accounts))
		for _, account := range accounts {
			check := accountCheck{
				stockAccount: account,
				Alerts:       stockAlerts([]stockAccount{account}, stockAlertsInput{}, day),
				AgentBatches: []agentBatch{},
			}
			if check.Alerts == nil {
				check.Alerts = []stockAlert{}
			}
			if reconciliationPath(account.ID, account.Program) != "" {
				if summary, err := s.statementSummary(ctx, who, account.ID); err != nil {
					check.StatementError = err.Error()
				} else {
					limit := in.MaxItems
					if limit <= 0 {
						limit = defaultDailyCheckItems
					}
					summary.truncate(limit)
					check.Statement = &summary
				}
			}
			if account.OwnsPointInventory {
				switch {
				case closingErr != nil:
					check.ClosingError = closingErr.Error()
				case closings[account.ID] != nil:
					check.Closing = closings[account.ID]
				}
				if batches, err := s.agentBatchesSince(ctx, who, account.ID, since); err == nil {
					check.AgentBatches = batches
				}
			}
			checks = append(checks, check)
		}

		lastReport := ""
		if !since.IsZero() {
			lastReport = since.Format(time.RFC3339)
		}
		return map[string]any{
			"date":           day.Format(time.DateOnly),
			"last_report_at": lastReport,
			"accounts":       checks,
		}, nil
	})

	addTool(s, server, &sdk.Tool{
		Name: "save_report",
		Description: "Grava o relatório do dia na página Agente MCP do painel: resumo, achados (com severidade) e ações tomadas. " +
			"Um relatório por dia por token; salvar de novo no mesmo dia substitui. Sempre permitido, mesmo para token de agente.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
	}, func(_ context.Context, who caller, in saveReportInput) (any, error) {
		if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Summary) == "" {
			return nil, fmt.Errorf("informe title e summary")
		}
		day, err := s.reportDay(in.ReportDate)
		if err != nil {
			return nil, err
		}
		for _, finding := range in.Findings {
			if finding.Severity != "info" && finding.Severity != "warning" && finding.Severity != "critical" {
				return nil, fmt.Errorf("severity deve ser info, warning ou critical (recebi %q)", finding.Severity)
			}
		}
		findings, err := json.Marshal(nonNil(in.Findings))
		if err != nil {
			return nil, err
		}
		actions, err := json.Marshal(nonNil(in.Actions))
		if err != nil {
			return nil, err
		}

		report := data.MCPReport{
			TokenID:    who.TokenID,
			ReportDate: day,
			Title:      in.Title,
			Summary:    in.Summary,
			Findings:   datatypes.JSON(findings),
			Actions:    datatypes.JSON(actions),
		}
		err = s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "token_id"}, {Name: "report_date"}},
			DoUpdates: clause.Assignments(map[string]any{"title": report.Title, "summary": report.Summary, "findings": report.Findings, "actions": report.Actions, "updated_at": s.now()}),
		}).Create(&report).Error
		if err != nil {
			return nil, fmt.Errorf("mcp: save report: %w", err)
		}
		return map[string]any{"saved": true, "report_date": day.Format(time.DateOnly), "findings": len(in.Findings), "actions": len(in.Actions)}, nil
	})

	addTool(s, server, &sdk.Tool{
		Name:        "list_reports",
		Description: "Relatórios já salvos por este token, mais recentes primeiro. Use antes da conferência para não repetir o que já foi reportado.",
		Annotations: readOnly(),
	}, func(_ context.Context, who caller, in struct {
		Limit int `json:"limit,omitempty" jsonschema:"Quantos (padrão 7, máximo 30)."`
	}) (any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 7
		}
		limit = min(limit, 30)
		var reports []data.MCPReport
		err := s.db.Where("token_id = ?", who.TokenID).Order("report_date desc").Limit(limit).Find(&reports).Error
		return reports, err
	})
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
