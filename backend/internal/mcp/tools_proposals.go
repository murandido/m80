package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/data"
)

type proposeCancelInput struct {
	IssuanceID       string  `json:"issuance_id" jsonschema:"ID (uuid) da emissão."`
	Reason           string  `json:"reason" jsonschema:"Por que cancelar (vai para quem aprova)."`
	RefundedPoints   *int    `json:"refunded_points,omitempty" jsonschema:"Pontos devolvidos pela cia (>= 0)."`
	RefundedTaxes    *int    `json:"refunded_taxes,omitempty" jsonschema:"Taxas devolvidas, em centavos (>= 0)."`
	CancellationFee  *int    `json:"cancellation_fee,omitempty" jsonschema:"Multa cobrada do cliente, em centavos (>= 0)."`
	CancelledOn      *string `json:"cancelled_on,omitempty" jsonschema:"Dia em que a cia devolveu os pontos (AAAA-MM-DD), quando não for hoje."`
	CancellationOnly bool    `json:"cancellation_only,omitempty" jsonschema:"A emissão existe só para carregar este estorno (reserva feita fora do painel)."`
}

type proposeIssuanceInput struct {
	IssuanceID string `json:"issuance_id" jsonschema:"ID (uuid) da emissão."`
	Reason     string `json:"reason" jsonschema:"Motivo (vai para quem aprova)."`
}

type proposeAdjustmentInput struct {
	IssuanceID   string `json:"issuance_id" jsonschema:"ID (uuid) da emissão."`
	AdjustmentID string `json:"adjustment_id" jsonschema:"ID da alteração."`
	Reason       string `json:"reason" jsonschema:"Motivo (vai para quem aprova)."`
}

type proposeBatchInput struct {
	AccountID string `json:"account_id" jsonschema:"ID da conta do lote."`
	BatchID   string `json:"batch_id" jsonschema:"ID do lote."`
	Reason    string `json:"reason" jsonschema:"Motivo (vai para quem aprova)."`
}

type proposeSettlementInput struct {
	AccountID string `json:"account_id" jsonschema:"ID da conta do lote."`
	BatchID   string `json:"batch_id" jsonschema:"ID do lote."`
	Settled   bool   `json:"settled" jsonschema:"true marca o lote como acertado entre as agências; false desfaz."`
	Reason    string `json:"reason" jsonschema:"Motivo (vai para quem aprova)."`
}

type proposeClosePeriodInput struct {
	AccountID string `json:"account_id" jsonschema:"ID da conta."`
	StartsOn  string `json:"starts_on" jsonschema:"Início do período (AAAA-MM-DD), logo após o último fechado."`
	EndsOn    string `json:"ends_on" jsonschema:"Fim do período (AAAA-MM-DD)."`
	Reason    string `json:"reason" jsonschema:"Motivo/observação (vai para quem aprova)."`
}

type proposeReopenPeriodInput struct {
	AccountID string `json:"account_id" jsonschema:"ID da conta."`
	PeriodID  string `json:"period_id" jsonschema:"ID do período (só o último fechado pode ser reaberto). Veja list_inventory_periods."`
	Reason    string `json:"reason" jsonschema:"Motivo (vai para quem aprova)."`
}

func requireReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("informe reason: quem aprova precisa saber por quê")
	}
	return nil
}

func bodyOf(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

// findByID picks the element with the given id out of a JSON list, for previews of resources
// that have no GET by id.
func findByID(raw json.RawMessage, id string) (map[string]any, error) {
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	for _, item := range items {
		if item["id"] == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("id %s não encontrado", id)
}

func (s *Server) issuancePreview(ctx context.Context, who caller, id string) (issuanceSummary, error) {
	var issuance apiIssuance
	if err := s.get(ctx, who, issuancePath(id), nil, &issuance); err != nil {
		return issuanceSummary{}, err
	}
	return issuance.summary(), nil
}

func (s *Server) batchPreview(ctx context.Context, who caller, accountID, batchID string) (map[string]any, error) {
	raw, err := s.call(ctx, who, http.MethodGet, "/point-batches", url.Values{"account_id": {accountID}}, nil)
	if err != nil {
		return nil, err
	}
	return findByID(raw, batchID)
}

const proposalNote = " Não executa nada: cria uma proposta que um dev aprova ou rejeita no painel em até 48h."

func (s *Server) registerProposalTools(server *sdk.Server) {
	destructive := &sdk.ToolAnnotations{DestructiveHint: boolPtr(true)}

	addTool(s, server, &sdk.Tool{
		Name:        "propose_cancel_issuance",
		Description: "Propõe cancelar uma emissão, com pontos e taxas devolvidos e multa (centavos). Ao aprovar, os pontos devolvidos voltam ao estoque e a venda é recalculada." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeCancelInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		preview, err := s.issuancePreview(ctx, who, in.IssuanceID)
		if err != nil {
			return nil, err
		}
		if preview.Status == "cancelled" {
			return nil, fmt.Errorf("a emissão %s já está cancelada", preview.ProgramLocator)
		}
		body := map[string]any{"cancellation_only": in.CancellationOnly}
		if in.RefundedPoints != nil {
			body["refunded_points"] = *in.RefundedPoints
		}
		if in.RefundedTaxes != nil {
			body["refunded_taxes"] = *in.RefundedTaxes
		}
		if in.CancellationFee != nil {
			body["cancellation_fee"] = *in.CancellationFee
		}
		if in.CancelledOn != nil {
			body["cancelled_on"] = *in.CancelledOn
		}
		summary := fmt.Sprintf("Cancelar emissão %s (%s, %d pontos): %s", preview.ProgramLocator, preview.Program, preview.Points, in.Reason)
		return s.propose(who, "propose_cancel_issuance", summary,
			Operation{Method: http.MethodPost, Path: issuancePath(in.IssuanceID, "cancel"), Body: bodyOf(body)}, preview)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "propose_uncancel_issuance",
		Description: "Propõe reverter o cancelamento de uma emissão (ela volta a consumir o estoque e a venda volta ao valor cheio)." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeIssuanceInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		preview, err := s.issuancePreview(ctx, who, in.IssuanceID)
		if err != nil {
			return nil, err
		}
		if preview.Status != "cancelled" {
			return nil, fmt.Errorf("a emissão %s não está cancelada", preview.ProgramLocator)
		}
		summary := fmt.Sprintf("Reverter cancelamento da emissão %s: %s", preview.ProgramLocator, in.Reason)
		return s.propose(who, "propose_uncancel_issuance", summary,
			Operation{Method: http.MethodPost, Path: issuancePath(in.IssuanceID, "uncancel")}, preview)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "propose_delete_issuance",
		Description: "Propõe excluir uma emissão (lançada por engano ou duplicada). Diferente de cancelar: some do painel e da venda." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeIssuanceInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		preview, err := s.issuancePreview(ctx, who, in.IssuanceID)
		if err != nil {
			return nil, err
		}
		summary := fmt.Sprintf("Excluir emissão %s (%s, %d pontos): %s", preview.ProgramLocator, preview.Program, preview.Points, in.Reason)
		return s.propose(who, "propose_delete_issuance", summary,
			Operation{Method: http.MethodDelete, Path: issuancePath(in.IssuanceID)}, preview)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "propose_delete_adjustment",
		Description: "Propõe excluir uma alteração de passagem de uma emissão." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeAdjustmentInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		var issuance struct {
			ProgramLocator string            `json:"program_locator"`
			Adjustments    []json.RawMessage `json:"adjustments"`
		}
		if err := s.get(ctx, who, issuancePath(in.IssuanceID), nil, &issuance); err != nil {
			return nil, err
		}
		adjustments, err := json.Marshal(issuance.Adjustments)
		if err != nil {
			return nil, err
		}
		preview, err := findByID(adjustments, in.AdjustmentID)
		if err != nil {
			return nil, fmt.Errorf("alteração não encontrada nesta emissão: %w", err)
		}
		summary := fmt.Sprintf("Excluir alteração da emissão %s: %s", issuance.ProgramLocator, in.Reason)
		return s.propose(who, "propose_delete_adjustment", summary,
			Operation{Method: http.MethodDelete, Path: issuancePath(in.IssuanceID, "adjustments", url.PathEscape(in.AdjustmentID))}, preview)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "propose_delete_point_batch",
		Description: "Propõe excluir um lote de pontos (as emissões que ele cobria passam a consumir o próximo lote, ou ficam pendentes)." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeBatchInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		preview, err := s.batchPreview(ctx, who, in.AccountID, in.BatchID)
		if err != nil {
			return nil, err
		}
		summary := fmt.Sprintf("Excluir lote de %v pontos de %v: %s", preview["points"], preview["acquired_on"], in.Reason)
		return s.propose(who, "propose_delete_point_batch", summary,
			Operation{Method: http.MethodDelete, Path: "/point-batches/" + url.PathEscape(in.BatchID)}, preview)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "propose_batch_settlement",
		Description: "Propõe marcar (ou desmarcar) um lote como acertado entre as agências." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeSettlementInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		preview, err := s.batchPreview(ctx, who, in.AccountID, in.BatchID)
		if err != nil {
			return nil, err
		}
		action := "Marcar como acertado"
		if !in.Settled {
			action = "Desfazer acerto do"
		}
		summary := fmt.Sprintf("%s lote de %v pontos de %v: %s", action, preview["points"], preview["acquired_on"], in.Reason)
		return s.propose(who, "propose_batch_settlement", summary,
			Operation{Method: http.MethodPatch, Path: "/point-batches/" + url.PathEscape(in.BatchID) + "/settlement", Body: bodyOf(map[string]bool{"settled": in.Settled})}, preview)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "propose_close_period",
		Description: "Propõe fechar o período de estoque de uma conta: congela o custeio FIFO do período. O preview guarda o fechamento (custo, receita, lucro, pendências) calculado agora." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeClosePeriodInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		closing, err := s.call(ctx, who, http.MethodGet, "/inventory-closing",
			url.Values{"account_ids": {in.AccountID}, "from": {in.StartsOn}, "to": {in.EndsOn}}, nil)
		if err != nil {
			return nil, err
		}
		summary := fmt.Sprintf("Fechar período %s a %s da conta %s: %s", in.StartsOn, in.EndsOn, in.AccountID, in.Reason)
		return s.propose(who, "propose_close_period", summary,
			Operation{Method: http.MethodPost, Path: "/inventory-periods", Body: bodyOf(map[string]string{
				"account_id": in.AccountID, "starts_on": in.StartsOn, "ends_on": in.EndsOn,
			})}, closing)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "propose_reopen_period",
		Description: "Propõe reabrir o último período fechado de uma conta, devolvendo-o ao recálculo." + proposalNote,
		Annotations: destructive,
	}, func(ctx context.Context, who caller, in proposeReopenPeriodInput) (any, error) {
		if err := requireReason(in.Reason); err != nil {
			return nil, err
		}
		periods, err := s.call(ctx, who, http.MethodGet, "/inventory-periods", url.Values{"account_id": {in.AccountID}}, nil)
		if err != nil {
			return nil, err
		}
		preview, err := findByID(periods, in.PeriodID)
		if err != nil {
			return nil, err
		}
		summary := fmt.Sprintf("Reabrir período %v a %v: %s", preview["starts_on"], preview["ends_on"], in.Reason)
		return s.propose(who, "propose_reopen_period", summary,
			Operation{Method: http.MethodDelete, Path: "/inventory-periods/" + url.PathEscape(in.PeriodID)}, preview)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "list_proposals",
		Description: "Propostas feitas por este token e o que foi decidido (pending, applied, rejected, failed, expired), com o resultado ou o erro.",
		Annotations: readOnly(),
	}, func(_ context.Context, who caller, in struct {
		Status string `json:"status,omitempty" jsonschema:"Filtra por status. Vazio traz todas."`
	}) (any, error) {
		if err := data.ExpireStaleMCPProposals(s.db, s.now()); err != nil {
			return nil, err
		}
		query := s.db.Where("user_id = ?", who.UserID).Order("created_at desc").Limit(50)
		if in.Status != "" {
			query = query.Where("status = ?", in.Status)
		}
		var proposals []data.MCPProposal
		if err := query.Find(&proposals).Error; err != nil {
			return nil, err
		}
		return proposals, nil
	})

	addTool(s, server, &sdk.Tool{
		Name:        "recent_mcp_calls",
		Description: "Últimas chamadas feitas com este token (ferramenta, argumentos, sucesso ou erro). Útil para conferir o que já foi feito numa rotina antes de repetir.",
		Annotations: readOnly(),
	}, func(_ context.Context, who caller, in struct {
		Hours int `json:"hours,omitempty" jsonschema:"Janela em horas. Padrão 24."`
	}) (any, error) {
		hours := in.Hours
		if hours <= 0 {
			hours = 24
		}
		var calls []data.MCPCall
		err := s.db.Where("token_id = ? AND created_at >= ?", who.TokenID, s.now().Add(-time.Duration(hours)*time.Hour)).
			Order("created_at desc").Limit(200).Find(&calls).Error
		return calls, err
	})
}
