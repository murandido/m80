package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// injunctionGroup is a group of the Controle de Liminares as the agent sees it: never the
// workbook link. The agency is per issuer: one workbook mixes issuers of different agencies.
type injunctionGroup struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	WhatsAppChatName *string `json:"whatsapp_chat_name"`
	HasSpreadsheet   bool    `json:"has_spreadsheet"`
	Accounts         []struct {
		AccountID    string `json:"account_id"`
		AccountLabel string `json:"account_label"`
		ProgramCode  string `json:"program_code"`
		Nicknames    string `json:"nicknames"`
	} `json:"accounts"`
	Issuers []struct {
		IssuerName string `json:"issuer_name"`
		UserName   string `json:"user_name"`
		AgencyName string `json:"agency_name"`
	} `json:"issuers"`
}

func (s *Server) injunctionGroups(ctx context.Context, who caller) ([]injunctionGroup, error) {
	var groups []injunctionGroup
	return groups, s.get(ctx, who, "/injunction-groups", nil, &groups)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type spreadsheetCheckInput struct {
	GroupID  string `json:"group_id" jsonschema:"ID do grupo de liminares (list_injunction_groups)."`
	From     string `json:"from,omitempty" jsonschema:"Primeiro dia da planilha (AAAA-MM-DD). Padrão: 7 dias atrás."`
	To       string `json:"to,omitempty" jsonschema:"Último dia (AAAA-MM-DD). Padrão: hoje."`
	MaxItems int    `json:"max_items,omitempty" jsonschema:"Máximo de itens por balde (padrão 30); os totais vêm sempre."`
}

// checkBuckets are the CheckResult lists, in the order the routine should read them.
var checkBuckets = []string{
	"missing_in_panel_confirmed", "missing_in_panel_unconfirmed", "differs_from_panel",
	"missing_in_sheet_and_panel", "pending_cancellations", "invalid", "needs_mapping",
}

type sheetRowRef struct {
	BlockIndex int `json:"block_index" jsonschema:"block_index do item em spreadsheet_check."`
	Number     int `json:"number" jsonschema:"number do item em spreadsheet_check."`
}

type importSheetInput struct {
	GroupID string        `json:"group_id" jsonschema:"ID do grupo de liminares."`
	Day     string        `json:"day" jsonschema:"Dia da aba da planilha (AAAA-MM-DD), o campo day do item."`
	Rows    []sheetRowRef `json:"rows" jsonschema:"As linhas a importar (block_index e number dos itens)."`
	Reason  string        `json:"reason,omitempty" jsonschema:"Por que importar (vai para a proposta, se virar uma)."`
}

func (s *Server) registerInjunctionTools(server *sdk.Server) {
	addTool(s, server, &sdk.Tool{
		Name: "list_injunction_groups",
		Description: "Controle de Liminares: grupos de contas liminares (ex.: as Tropas) que controlam as emissões numa planilha do OneDrive e conversam com a agência num grupo de WhatsApp. " +
			"Para cada grupo: nome do grupo do WhatsApp, contas (id, programa, rótulo da planilha) e emissores mapeados, cada um com a agência das emissões dele. Comece a rotina por aqui.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, _ noInput) (any, error) {
		return s.injunctionGroups(ctx, who)
	})

	addTool(s, server, &sdk.Tool{
		Name: "spreadsheet_check",
		Description: "Baixa a planilha do grupo e cruza com o painel e o extrato, só nas contas liminares dele (taxa de resgate ignorada). Baldes: " +
			"missing_in_panel_confirmed (planilha e extrato concordam, falta no painel: pode importar), missing_in_panel_unconfirmed (só a planilha), " +
			"differs_from_panel (pontos/milheiro/conta diferentes), missing_in_sheet_and_panel (só o extrato), pending_cancellations (estornos), " +
			"invalid (localizador sem dados) e needs_mapping. Pontos inteiros; milheiro em centavos por mil pontos.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in spreadsheetCheckInput) (any, error) {
		query := url.Values{}
		if in.From != "" {
			query.Set("from", in.From)
		}
		if in.To != "" {
			query.Set("to", in.To)
		}
		raw, err := s.call(ctx, who, http.MethodGet, "/injunction-groups/"+url.PathEscape(in.GroupID)+"/check", query, nil)
		if err != nil {
			return nil, err
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, err
		}
		limit := in.MaxItems
		if limit <= 0 {
			limit = defaultDailyCheckItems
		}
		totals := map[string]int{}
		for _, bucket := range checkBuckets {
			var items []json.RawMessage
			if err := json.Unmarshal(result[bucket], &items); err != nil {
				continue
			}
			totals[bucket] = len(items)
			if len(items) > limit {
				items = items[:limit]
			}
			trimmed, err := json.Marshal(items)
			if err != nil {
				return nil, err
			}
			result[bucket] = trimmed
		}
		totalsJSON, err := json.Marshal(totals)
		if err != nil {
			return nil, err
		}
		result["totals"] = totalsJSON
		return result, nil
	})

	addTool(s, server, &sdk.Tool{
		Name: "import_spreadsheet_issuances",
		Description: "Importa linhas de um dia da planilha do grupo como emissões no painel (estornos cancelam a emissão do localizador). " +
			"Só linhas das contas do grupo. Token de agente: grava direto apenas se todas as linhas estiverem em missing_in_panel_confirmed " +
			"(planilha e extrato concordam); senão vira proposta.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, func(ctx context.Context, who caller, in importSheetInput) (any, error) {
		if len(in.Rows) == 0 {
			return nil, fmt.Errorf("informe rows: as linhas a importar, com block_index e number de spreadsheet_check")
		}
		body := map[string]any{"day": in.Day, "rows": in.Rows}

		var reasons []string
		if who.Policy != nil {
			// Re-check the day now: the routine's earlier check may be minutes old, and only
			// what the statement backs right now may be written without a person.
			query := url.Values{"from": {in.Day}, "to": {in.Day}}
			var check struct {
				Confirmed []struct {
					BlockIndex int `json:"block_index"`
					Number     int `json:"number"`
				} `json:"missing_in_panel_confirmed"`
			}
			if err := s.get(ctx, who, "/injunction-groups/"+url.PathEscape(in.GroupID)+"/check", query, &check); err != nil {
				return nil, err
			}
			confirmed := map[sheetRowRef]bool{}
			for _, row := range check.Confirmed {
				confirmed[sheetRowRef{BlockIndex: row.BlockIndex, Number: row.Number}] = true
			}
			for _, row := range in.Rows {
				if !confirmed[row] {
					reasons = append(reasons, fmt.Sprintf("linha %d sem confirmação do extrato", row.Number))
				}
			}
		}

		summary := fmt.Sprintf("Importar %d linha(s) da planilha do grupo, aba %s", len(in.Rows), in.Day)
		if in.Reason != "" {
			summary += ": " + in.Reason
		}
		return s.writeOrPropose(ctx, who, "import_spreadsheet_issuances", writeIntent{
			Operation: Operation{Method: http.MethodPost, Path: "/injunction-groups/" + url.PathEscape(in.GroupID) + "/import", Body: bodyOf(body)},
			Summary:   summary,
			Preview:   func() (any, error) { return in, nil },
			Reasons:   reasons,
		})
	})
}
