package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxResultBytes keeps one tool answer from flooding the model's context. A listing that big
// is a query that needs a filter, and the truncation note says so.
const maxResultBytes = 200_000

type searchIssuancesInput struct {
	Q                string `json:"q,omitempty" jsonschema:"localizador (programa ou cia), passageiro, origem ou destino; com busca, os filtros de data e emissor são ignorados"`
	DateFrom         string `json:"date_from,omitempty" jsonschema:"data inicial da emissão, AAAA-MM-DD, inclusiva"`
	DateTo           string `json:"date_to,omitempty" jsonschema:"data final da emissão, AAAA-MM-DD, inclusiva"`
	LoyaltyProgramID string `json:"loyalty_program_id,omitempty" jsonschema:"UUID do programa de fidelidade"`
	AirlineID        string `json:"airline_id,omitempty" jsonschema:"UUID da companhia aérea"`
	AccountID        string `json:"account_id,omitempty" jsonschema:"UUID da conta de fidelidade usada"`
	AgencyID         string `json:"agency_id,omitempty" jsonschema:"UUID da agência"`
	IssuerID         string `json:"issuer_id,omitempty" jsonschema:"UUID do emissor; vários separados por vírgula; 'all' para todos"`
}

type idInput struct {
	ID string `json:"id" jsonschema:"UUID do registro"`
}

type listInput struct {
	Q     string `json:"q,omitempty" jsonschema:"texto de busca"`
	Page  int    `json:"page,omitempty" jsonschema:"página, a partir de 1"`
	Limit int    `json:"limit,omitempty" jsonschema:"itens por página, até 100 (padrão 20)"`
}

type searchClientsInput struct {
	listInput
	AgencyID string `json:"agency_id,omitempty" jsonschema:"UUID da agência"`
}

type listAccountsInput struct {
	listInput
	LoyaltyProgramID string `json:"loyalty_program_id,omitempty" jsonschema:"UUID do programa de fidelidade"`
	Category         string `json:"category,omitempty" jsonschema:"categoria da conta"`
}

type listSalesInput struct {
	Q        string `json:"q,omitempty" jsonschema:"texto de busca (localizador, passageiro)"`
	ClientID string `json:"client_id,omitempty" jsonschema:"UUID do cliente"`
	AgencyID string `json:"agency_id,omitempty" jsonschema:"UUID da agência"`
	Paid     string `json:"paid,omitempty" jsonschema:"true para só pagas, false para só em aberto"`
	Offset   int    `json:"offset,omitempty" jsonschema:"deslocamento da página"`
	Limit    int    `json:"limit,omitempty" jsonschema:"itens por página"`
}

type pointBatchesInput struct {
	AccountID string `json:"account_id" jsonschema:"UUID da conta (obrigatório: os lotes são listados por conta)"`
	From      string `json:"from,omitempty" jsonschema:"data inicial, AAAA-MM-DD"`
	To        string `json:"to,omitempty" jsonschema:"data final, AAAA-MM-DD"`
}

type inventoryClosingInput struct {
	From       string `json:"from,omitempty" jsonschema:"data inicial do período, AAAA-MM-DD"`
	To         string `json:"to,omitempty" jsonschema:"data final do período, AAAA-MM-DD"`
	AccountIDs string `json:"account_ids,omitempty" jsonschema:"UUIDs de contas separados por vírgula"`
}

type apiGetInput struct {
	Path  string            `json:"path" jsonschema:"caminho sob /api/v1, começando com /, ex. /issuances/{id}/passengers"`
	Query map[string]string `json:"query,omitempty" jsonschema:"parâmetros de query"`
}

func addReadTools(s *mcp.Server, api loopback) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "whoami",
		Description: "Quem é o dono do token: nome, papel (user/admin/dev) e agências que enxerga.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return respond(api.call(ctx, "GET", "/auth/me", nil, nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "search_issuances",
		Description: "Lista emissões (passagens emitidas). Sem filtros, traz a janela padrão do painel. " +
			"Para achar uma reserva, use q com o localizador.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchIssuancesInput) (*mcp.CallToolResult, any, error) {
		return respond(api.call(ctx, "GET", "/issuances", queryOf(map[string]string{
			"q": in.Q, "date_from": in.DateFrom, "date_to": in.DateTo,
			"loyalty_program_id": in.LoyaltyProgramID, "airline_id": in.AirlineID,
			"account_id": in.AccountID, "agency_id": in.AgencyID, "issuer_id": in.IssuerID,
		}), nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_issuance",
		Description: "Uma emissão completa pelo id: venda, custo, passageiros, ajustes e cancelamento.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		if err := requireUUID(in.ID); err != nil {
			return nil, nil, err
		}
		return respond(api.call(ctx, "GET", "/issuances/"+in.ID, nil, nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_clients",
		Description: "Busca clientes por nome, telefone ou localizador de uma emissão deles.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchClientsInput) (*mcp.CallToolResult, any, error) {
		return respond(api.call(ctx, "GET", "/clients", withPage(in.listInput, map[string]string{"agency_id": in.AgencyID}), nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_accounts",
		Description: "Contas de fidelidade (Azul, Smiles, LATAM...) com titular, saldo real lido no último sync " +
			"e estado do sync.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listAccountsInput) (*mcp.CallToolResult, any, error) {
		return respond(api.call(ctx, "GET", "/accounts", withPage(in.listInput, map[string]string{
			"loyalty_program_id": in.LoyaltyProgramID, "category": in.Category,
		}), nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_sales",
		Description: "Vendas (o lado financeiro de cada emissão): valor, pago, em aberto. Paginado por offset/limit.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listSalesInput) (*mcp.CallToolResult, any, error) {
		return respond(api.call(ctx, "GET", "/sales/page", queryOf(map[string]string{
			"q": in.Q, "client_id": in.ClientID, "agency_id": in.AgencyID, "paid": in.Paid,
			"offset": intParam(in.Offset), "limit": intParam(in.Limit),
		}), nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_point_batches",
		Description: "Lotes de pontos de uma conta (custeio FIFO): compra, custo por milheiro, consumo. Só admin.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pointBatchesInput) (*mcp.CallToolResult, any, error) {
		if err := requireUUID(in.AccountID); err != nil {
			return nil, nil, err
		}
		return respond(api.call(ctx, "GET", "/point-batches", queryOf(map[string]string{
			"account_id": in.AccountID, "from": in.From, "to": in.To,
		}), nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "inventory_closing",
		Description: "Relatório de fechamento do estoque de pontos num período: custo, lucro e rateio por lote. Só admin.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in inventoryClosingInput) (*mcp.CallToolResult, any, error) {
		return respond(api.call(ctx, "GET", "/inventory-closing", queryOf(map[string]string{
			"from": in.From, "to": in.To, "account_ids": in.AccountIDs,
		}), nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "sync_status",
		Description: "Estado da leitura automática de extrato/saldo de uma conta: Azul, Smiles e LATAM, " +
			"com último sync, falhas seguidas e o erro. Só admin.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		if err := requireUUID(in.ID); err != nil {
			return nil, nil, err
		}
		// An account belongs to one program, so two of the three answer "not this program";
		// collecting all three spares the caller from knowing which one applies.
		result := map[string]any{}
		for program, path := range map[string]string{
			"azul":   "/accounts/" + in.ID + "/azul-statement/status",
			"smiles": "/accounts/" + in.ID + "/smiles-statement/status",
			"latam":  "/accounts/" + in.ID + "/latam-status",
		} {
			raw, err := api.call(ctx, "GET", path, nil, nil)
			if err != nil {
				result[program] = map[string]string{"erro": err.Error()}
				continue
			}
			result[program] = raw
		}
		encoded, err := json.Marshal(result)
		return respond(encoded, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "system_status",
		Description: "Saúde das dependências: bot, Traveos e o provedor de reserva de cada companhia.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return respond(api.call(ctx, "GET", "/status", nil, nil))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "api_get",
		Description: "Leitura genérica de qualquer GET JSON da API v1 (/api/v1), com as mesmas permissões do painel. " +
			"Use quando nenhuma ferramenta específica cobre o caso, ex. /issuances/{id}/passengers, " +
			"/sales/{id}/payments, /sales/client-payments, /inventory-periods, /agencies, /loyalty-programs. " +
			"Arquivos (export, pdf) e o stream /events não funcionam aqui.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in apiGetInput) (*mcp.CallToolResult, any, error) {
		if err := validateAPIPath(in.Path); err != nil {
			return nil, nil, err
		}
		return respond(api.call(ctx, "GET", in.Path, queryOf(in.Query), nil))
	})
}

// validateAPIPath keeps api_get on plain resource reads. The router would reject most of the
// rest anyway; the stream is the one that would not fail but hang until the timeout.
func validateAPIPath(path string) error {
	switch {
	case !strings.HasPrefix(path, "/"):
		return errors.New("path deve começar com /")
	case strings.Contains(path, "..") || strings.ContainsAny(path, "?#"):
		return errors.New("path não pode ter '..', '?' ou '#'; use o campo query")
	case path == "/events" || strings.HasPrefix(path, "/events/"):
		return errors.New("/events é um stream SSE e não pode ser lido por aqui")
	}
	return nil
}

func requireUUID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("id inválido %q: esperado UUID", id)
	}
	return nil
}

func queryOf(params map[string]string) url.Values {
	query := url.Values{}
	for key, value := range params {
		if value = strings.TrimSpace(value); value != "" {
			query.Set(key, value)
		}
	}
	return query
}

func withPage(page listInput, extra map[string]string) url.Values {
	extra["q"] = page.Q
	extra["page"] = intParam(page.Page)
	extra["limit"] = intParam(page.Limit)
	return queryOf(extra)
}

func intParam(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprint(value)
}

// respond turns a loopback answer into a tool result. An API error is a tool error the model
// reads (IsError), not a protocol failure: "not found" or "admin only" is an answer.
func respond(raw json.RawMessage, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	text := string(raw)
	if text == "" {
		text = "null"
	}
	if len(text) > maxResultBytes {
		text = text[:maxResultBytes] + "\n…[resposta truncada: refine os filtros ou pagine]"
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}
