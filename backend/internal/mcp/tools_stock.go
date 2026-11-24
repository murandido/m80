package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/inventory"
)

// stockAccount is the slice of GET /accounts the agent gets. It is a projection on purpose:
// the endpoint returns each account's password decrypted, for the staff who log into the
// programs by hand, and that must never reach the agent.
type stockAccount struct {
	ID                    string         `json:"id"`
	Program               string         `json:"program"`
	ProgramName           string         `json:"program_name"`
	Nicknames             []string       `json:"nicknames"`
	Category              string         `json:"category"`
	OwnsPointInventory    bool           `json:"owns_point_inventory"`
	ExpectedBalancePoints *int           `json:"expected_balance_points"`
	RealBalancePoints     *int           `json:"real_balance_points"`
	RealBalanceCheckedOn  *string        `json:"real_balance_checked_on"`
	DivergencePoints      *int           `json:"divergence_points,omitempty"`
	Sync                  *syncState     `json:"sync,omitempty"`
	CPFUsage              map[string]any `json:"cpf_usage,omitempty"`
}

type syncState struct {
	Connected    bool       `json:"connected"`
	State        *string    `json:"state"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
}

// apiAccount is what is read off GET /accounts. Password and login are left out of the struct,
// so they are dropped at decoding and never held in memory past it.
type apiAccount struct {
	ID             string `json:"id"`
	LoyaltyProgram struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Code string `json:"code"`
	} `json:"loyalty_program"`
	Nicknames []struct {
		Nickname string `json:"nickname"`
	} `json:"nicknames"`
	Category              string         `json:"category"`
	OwnsPointInventory    bool           `json:"owns_point_inventory"`
	RealBalancePoints     *int           `json:"real_balance_points"`
	RealBalanceCheckedOn  *string        `json:"real_balance_checked_on"`
	ExpectedBalancePoints *int           `json:"expected_balance_points"`
	AzulConnected         bool           `json:"azul_connected"`
	AzulLastSyncedAt      *time.Time     `json:"azul_last_synced_at"`
	AzulSyncState         *string        `json:"azul_sync_state"`
	SmilesConnected       bool           `json:"smiles_connected"`
	SmilesLastSyncedAt    *time.Time     `json:"smiles_last_synced_at"`
	LatamConnected        bool           `json:"latam_connected"`
	LatamLastSyncedAt     *time.Time     `json:"latam_last_synced_at"`
	LatamSyncState        *string        `json:"latam_sync_state"`
	CPFUsage              map[string]any `json:"cpf_usage"`
}

func (a apiAccount) toStock() stockAccount {
	out := stockAccount{
		ID:                    a.ID,
		Program:               a.LoyaltyProgram.Code,
		ProgramName:           a.LoyaltyProgram.Name,
		Nicknames:             make([]string, 0, len(a.Nicknames)),
		Category:              a.Category,
		OwnsPointInventory:    a.OwnsPointInventory,
		ExpectedBalancePoints: a.ExpectedBalancePoints,
		RealBalancePoints:     a.RealBalancePoints,
		RealBalanceCheckedOn:  a.RealBalanceCheckedOn,
		CPFUsage:              a.CPFUsage,
	}
	for _, n := range a.Nicknames {
		out.Nicknames = append(out.Nicknames, n.Nickname)
	}
	if a.ExpectedBalancePoints != nil && a.RealBalancePoints != nil {
		divergence := *a.RealBalancePoints - *a.ExpectedBalancePoints
		out.DivergencePoints = &divergence
	}
	switch a.LoyaltyProgram.Code {
	case "AZUL":
		out.Sync = &syncState{Connected: a.AzulConnected, State: a.AzulSyncState, LastSyncedAt: a.AzulLastSyncedAt}
	case "SMILES":
		// The Smiles sync has no state field on the account; connected and last sync are all
		// the panel shows either.
		out.Sync = &syncState{Connected: a.SmilesConnected, LastSyncedAt: a.SmilesLastSyncedAt}
	case "LATAM":
		out.Sync = &syncState{Connected: a.LatamConnected, State: a.LatamSyncState, LastSyncedAt: a.LatamLastSyncedAt}
	}
	return out
}

func (a stockAccount) label() string {
	if len(a.Nicknames) > 0 {
		return a.Program + " " + strings.Join(a.Nicknames, "/")
	}
	return a.Program + " " + a.ID
}

func (s *Server) listStockAccounts(ctx context.Context, who caller, program, category string) ([]stockAccount, error) {
	var query url.Values
	if category = strings.TrimSpace(category); category != "" {
		query = url.Values{"category": {category}}
	}
	var accounts []apiAccount
	if err := s.get(ctx, who, "/accounts", query, &accounts); err != nil {
		return nil, err
	}
	program = strings.ToUpper(strings.TrimSpace(program))
	out := make([]stockAccount, 0, len(accounts))
	for _, a := range accounts {
		if program != "" && a.LoyaltyProgram.Code != program {
			continue
		}
		out = append(out, a.toStock())
	}
	return out, nil
}

type stockOverviewInput struct {
	Program       string `json:"program,omitempty" jsonschema:"Código do programa para filtrar: SMILES, LATAM, AZUL... Vazio traz todos."`
	InventoryOnly bool   `json:"inventory_only,omitempty" jsonschema:"Só contas com estoque próprio (owns_point_inventory), as que têm saldo esperado."`
	Category      string `json:"category,omitempty" jsonschema:"Categoria da conta: injunction (liminar), standard ou unidentified. Vazio traz todas."`
}

type programTotals struct {
	Program               string         `json:"program"`
	Accounts              int            `json:"accounts"`
	ExpectedBalancePoints int            `json:"expected_balance_points"`
	RealBalancePoints     int            `json:"real_balance_points"`
	AccountList           []stockAccount `json:"account_list"`
}

type stockAlertsInput struct {
	DivergenceThresholdPoints int    `json:"divergence_threshold_points,omitempty" jsonschema:"Diferença mínima entre saldo real e esperado para alertar, em pontos. Padrão 1000."`
	StaleAfterDays            int    `json:"stale_after_days,omitempty" jsonschema:"Saldo real lido há mais dias que isto é considerado desatualizado. Padrão 3."`
	CPFRemainingThreshold     int    `json:"cpf_remaining_threshold,omitempty" jsonschema:"Alerta quando restam esta quantidade ou menos de CPFs no limite do programa. Padrão 2."`
	Category                  string `json:"category,omitempty" jsonschema:"Só contas desta categoria: injunction (liminar), standard ou unidentified. Vazio traz todas."`
}

type stockAlert struct {
	AccountID string `json:"account_id"`
	Account   string `json:"account"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail"`
}

// Sync states that need a person: everything except a healthy or still-starting sync.
var healthySyncStates = map[string]bool{"ok": true, "pending": true}

func stockAlerts(accounts []stockAccount, in stockAlertsInput, today time.Time) []stockAlert {
	threshold := in.DivergenceThresholdPoints
	if threshold <= 0 {
		threshold = 1000
	}
	staleDays := in.StaleAfterDays
	if staleDays <= 0 {
		staleDays = 3
	}
	cpfThreshold := in.CPFRemainingThreshold
	if cpfThreshold <= 0 {
		cpfThreshold = 2
	}

	var alerts []stockAlert
	add := func(a stockAccount, kind, detail string) {
		alerts = append(alerts, stockAlert{AccountID: a.ID, Account: a.label(), Kind: kind, Detail: detail})
	}

	for _, a := range accounts {
		if a.ExpectedBalancePoints != nil && *a.ExpectedBalancePoints < 0 {
			add(a, "negative_expected_balance", fmt.Sprintf("saldo esperado negativo (%d pontos): emissões sem lote que as cubra", *a.ExpectedBalancePoints))
		}
		if a.DivergencePoints != nil && abs(*a.DivergencePoints) >= threshold {
			add(a, "balance_divergence", fmt.Sprintf("saldo real %d × esperado %d (diferença %+d pontos)", *a.RealBalancePoints, *a.ExpectedBalancePoints, *a.DivergencePoints))
		}
		if a.Sync != nil && a.Sync.Connected {
			if a.RealBalanceCheckedOn == nil {
				add(a, "balance_never_read", "conta conectada sem nenhum saldo real lido")
			} else if checked, err := time.Parse(time.DateOnly, *a.RealBalanceCheckedOn); err == nil && today.Sub(checked) > time.Duration(staleDays)*24*time.Hour {
				add(a, "stale_balance", "saldo real lido pela última vez em "+*a.RealBalanceCheckedOn)
			}
			if a.Sync.State != nil && !healthySyncStates[*a.Sync.State] {
				add(a, "sync_needs_attention", "sincronização em estado "+*a.Sync.State)
			}
		}
		if remaining, ok := a.CPFUsage["remaining"].(float64); ok && int(remaining) <= cpfThreshold {
			add(a, "cpf_limit", fmt.Sprintf("restam %d CPFs no limite do programa (%v usados de %v)", int(remaining), a.CPFUsage["used"], a.CPFUsage["limit"]))
		}
	}
	return alerts
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type accountIDInput struct {
	AccountID string `json:"account_id" jsonschema:"ID (uuid) da conta. Veja stock_overview."`
}

type listBatchesInput struct {
	AccountID string `json:"account_id" jsonschema:"ID (uuid) da conta."`
	From      string `json:"from,omitempty" jsonschema:"Início (AAAA-MM-DD) pela data de aquisição. Vazio = desde o começo."`
	To        string `json:"to,omitempty" jsonschema:"Fim (AAAA-MM-DD) pela data de aquisição. Vazio = até hoje."`
}

type addBatchInput struct {
	AccountID          string       `json:"account_id" jsonschema:"ID (uuid) da conta que recebe os pontos. Precisa ter estoque próprio."`
	OriginID           *string      `json:"origin_id,omitempty" jsonschema:"ID da origem (compra, transferência, bônus...). Veja list_point_batch_origins. Obrigatório, salvo em saldo de abertura."`
	AcquiredOn         string       `json:"acquired_on" jsonschema:"Dia em que os pontos entraram (AAAA-MM-DD)."`
	Points             int          `json:"points" jsonschema:"Quantidade de pontos, inteiro positivo (50 mil = 50000)."`
	CostCents          int          `json:"cost_cents" jsonschema:"Custo total do lote em centavos (R$ 894,72 = 89472). Zero para bônus."`
	FundingAgencyID    string       `json:"funding_agency_id" jsonschema:"ID da agência que pagou pelos pontos. Veja list_agencies."`
	ProfitSharePercent *int         `json:"profit_share_percent,omitempty" jsonschema:"Divisão de lucro deste lote (0-100). Vazio usa o padrão da conta."`
	Opening            bool         `json:"opening,omitempty" jsonschema:"Saldo de abertura (carregado de antes), não uma compra."`
	Note               *string      `json:"note,omitempty" jsonschema:"Observação livre."`
	Source             *batchSource `json:"source,omitempty" jsonschema:"De onde veio o lote (obrigatório para tokens de agente com política de escrita)."`
}

// body is the request the API takes: the source goes into the note, not into a field.
func (in addBatchInput) body() addBatchInput {
	out := in
	if in.Source != nil {
		note := in.Source.note(in.Note)
		out.Note = &note
	}
	out.Source = nil
	return out
}

type updateBatchInput struct {
	BatchID string            `json:"batch_id" jsonschema:"ID do lote. Veja list_point_batches."`
	Changes updateBatchFields `json:"changes" jsonschema:"Só os campos que mudam."`
}

type updateBatchFields struct {
	OriginID           *string `json:"origin_id,omitempty" jsonschema:"ID da origem."`
	AcquiredOn         *string `json:"acquired_on,omitempty" jsonschema:"AAAA-MM-DD."`
	Points             *int    `json:"points,omitempty" jsonschema:"Pontos, inteiro positivo."`
	CostCents          *int    `json:"cost_cents,omitempty" jsonschema:"Custo total em centavos."`
	FundingAgencyID    *string `json:"funding_agency_id,omitempty" jsonschema:"ID da agência que pagou."`
	ProfitSharePercent *int    `json:"profit_share_percent,omitempty" jsonschema:"0-100."`
	Note               *string `json:"note,omitempty" jsonschema:"Observação livre."`
}

type closingInput struct {
	AccountIDs []string `json:"account_ids" jsonschema:"IDs das contas (uma ou mais) do fechamento."`
	From       string   `json:"from" jsonschema:"Início do período (AAAA-MM-DD)."`
	To         string   `json:"to" jsonschema:"Fim do período (AAAA-MM-DD)."`
}

func (s *Server) registerStockTools(server *sdk.Server) {
	addTool(s, server, &sdk.Tool{
		Name: "stock_overview",
		Description: "Estoque de pontos por conta, agrupado por programa: saldo esperado (lotes menos o que foi emitido; só em contas com estoque próprio), " +
			"saldo real lido da cia e quando, divergência (real menos esperado), estado da sincronização e uso do limite de CPFs. Pontos inteiros.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in stockOverviewInput) (any, error) {
		accounts, err := s.listStockAccounts(ctx, who, in.Program, in.Category)
		if err != nil {
			return nil, err
		}
		byProgram := map[string]*programTotals{}
		for _, a := range accounts {
			if in.InventoryOnly && !a.OwnsPointInventory {
				continue
			}
			totals := byProgram[a.Program]
			if totals == nil {
				totals = &programTotals{Program: a.Program}
				byProgram[a.Program] = totals
			}
			totals.Accounts++
			if a.ExpectedBalancePoints != nil {
				totals.ExpectedBalancePoints += *a.ExpectedBalancePoints
			}
			if a.RealBalancePoints != nil {
				totals.RealBalancePoints += *a.RealBalancePoints
			}
			totals.AccountList = append(totals.AccountList, a)
		}
		out := make([]*programTotals, 0, len(byProgram))
		for _, totals := range byProgram {
			out = append(out, totals)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Program < out[j].Program })
		return out, nil
	})

	addTool(s, server, &sdk.Tool{
		Name: "stock_alerts",
		Description: "Lista o que precisa de atenção no estoque: saldo esperado negativo, divergência entre saldo real e esperado, saldo real desatualizado, " +
			"sincronização com problema (pedir reconexão/login) e contas perto do limite de CPFs. Feita para rodar em rotina; lista vazia = tudo em ordem.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in stockAlertsInput) (any, error) {
		accounts, err := s.listStockAccounts(ctx, who, "", in.Category)
		if err != nil {
			return nil, err
		}
		alerts := stockAlerts(accounts, in, inventory.DayOf(s.now()))
		if alerts == nil {
			alerts = []stockAlert{}
		}
		return map[string]any{"checked_accounts": len(accounts), "alerts": alerts}, nil
	})

	addTool(s, server, &sdk.Tool{
		Name:        "list_point_batches",
		Description: "Lotes de pontos de uma conta (compras, transferências, bônus, saldo de abertura), mais recentes primeiro, com custo total em centavos e custo do milheiro (centavos por mil pontos). Só lotes pagos por agências visíveis.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in listBatchesInput) (any, error) {
		query := url.Values{"account_id": {in.AccountID}}
		if in.From != "" {
			query.Set("from", in.From)
		}
		if in.To != "" {
			query.Set("to", in.To)
		}
		var batches any
		return batches, s.get(ctx, who, "/point-batches", query, &batches)
	})

	addTool(s, server, &sdk.Tool{
		Name: "add_point_batch",
		Description: "Lança um lote de pontos (entrada no estoque) numa conta com estoque próprio. Recusado se a data cair num período já fechado. " +
			"Token de agente (com política): exige source; só grava direto se o extrato mostrar um crédito sem lote com a mesma data e pontos, " +
			"sem lote igual já lançado e dentro dos limites do token; senão vira proposta. Prefira record_statement_batches quando houver extrato.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, func(ctx context.Context, who caller, in addBatchInput) (any, error) {
		if err := requireSource(who, in.Source); err != nil {
			return nil, err
		}
		reasons, err := s.batchReasons(who, in.Points, 1)
		if err != nil {
			return nil, err
		}
		if who.Policy != nil {
			// An agent records a batch by itself only when the statement already shows the
			// credit; everything else needs a human to confirm the points exist.
			if duplicate, err := s.duplicateBatch(ctx, who, in.AccountID, in.AcquiredOn, in.Points); err != nil {
				return nil, err
			} else if duplicate {
				reasons = append(reasons, "já existe lote com a mesma data e os mesmos pontos")
			}
			matched, err := s.statementMatch(ctx, who, in.AccountID, in.AcquiredOn, in.Points)
			if err != nil || !matched {
				reasons = append(reasons, "o extrato não mostra um crédito sem lote com essa data e esses pontos")
			}
		}
		return s.writeOrPropose(ctx, who, "add_point_batch", writeIntent{
			Operation: Operation{Method: http.MethodPost, Path: "/point-batches", Body: bodyOf(in.body())},
			Summary:   fmt.Sprintf("Lançar lote de %d pontos em %s (custo %d centavos)", in.Points, in.AcquiredOn, in.CostCents),
			Preview:   func() (any, error) { return in, nil },
			Reasons:   reasons,
		})
	})

	addTool(s, server, &sdk.Tool{
		Name:        "update_point_batch",
		Description: "Corrige um lote de pontos (data, pontos, custo, origem, agência, divisão, observação). Grava direto, salvo limite do token. Mudar de conta não existe: exclua (proposta) e lance de novo.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
	}, func(ctx context.Context, who caller, in updateBatchInput) (any, error) {
		var reasons []string
		if in.Changes.Points != nil {
			// A correction can add points as surely as a new batch can, so the same ceiling
			// applies; it is not a new batch, so the daily count is left alone.
			var err error
			if reasons, err = s.batchReasons(who, *in.Changes.Points, 0); err != nil {
				return nil, err
			}
		}
		return s.writeOrPropose(ctx, who, "update_point_batch", writeIntent{
			Operation: Operation{Method: http.MethodPut, Path: "/point-batches/" + url.PathEscape(in.BatchID), Body: bodyOf(in.Changes)},
			Summary:   "Corrigir lote " + in.BatchID,
			Preview:   func() (any, error) { return in.Changes, nil },
			Reasons:   reasons,
		})
	})

	addTool(s, server, &sdk.Tool{
		Name: "inventory_closing",
		Description: "Fechamento do estoque de uma ou mais contas num período: consumo FIFO por lote, custo, receita e lucro (centavos), pontos pendentes (emitidos sem lote que os cubra) " +
			"e acertos entre agências. Só leitura: fechar o período é propose_close_period.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in closingInput) (any, error) {
		query := url.Values{"account_ids": {strings.Join(in.AccountIDs, ",")}, "from": {in.From}, "to": {in.To}}
		var closing any
		return closing, s.get(ctx, who, "/inventory-closing", query, &closing)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "list_inventory_periods",
		Description: "Períodos de estoque já fechados de uma conta. Nada dentro de um período fechado pode ser lançado ou alterado sem reabri-lo.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in accountIDInput) (any, error) {
		var periods any
		return periods, s.get(ctx, who, "/inventory-periods", url.Values{"account_id": {in.AccountID}}, &periods)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "account_statement_status",
		Description: "Estado da conexão e da leitura de extrato de uma conta Azul ou Smiles. LATAM não tem extrato: o estado dela já vem em stock_overview.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in accountIDInput) (any, error) {
		var account apiAccount
		if err := s.get(ctx, who, "/accounts/"+url.PathEscape(in.AccountID), nil, &account); err != nil {
			return nil, err
		}
		var path string
		switch account.LoyaltyProgram.Code {
		case "AZUL":
			path = "/accounts/" + url.PathEscape(in.AccountID) + "/azul-statement/status"
		case "SMILES":
			path = "/accounts/" + url.PathEscape(in.AccountID) + "/smiles-statement/status"
		default:
			return nil, fmt.Errorf("a conta é do programa %s, que não tem leitura de extrato", account.LoyaltyProgram.Code)
		}
		var status any
		return status, s.get(ctx, who, path, nil, &status)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "list_point_batch_origins",
		Description: "Origens possíveis de um lote de pontos (compra, transferência de banco, bônus...), com o id usado em add_point_batch.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, _ noInput) (any, error) {
		var origins any
		return origins, s.get(ctx, who, "/point-batch-origins", nil, &origins)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "list_loyalty_programs",
		Description: "Programas de fidelidade (Smiles, LATAM Pass, Azul...) com id, código, cia padrão e regra de limite de CPFs.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, _ noInput) (any, error) {
		var programs any
		return programs, s.get(ctx, who, "/loyalty-programs", nil, &programs)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "list_agencies",
		Description: "Agências visíveis para o dono do token, com id (usado como funding_agency_id e agency_id).",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, _ noInput) (any, error) {
		var agencies any
		return agencies, s.get(ctx, who, "/agencies", nil, &agencies)
	})
}

func boolPtr(b bool) *bool { return &b }
