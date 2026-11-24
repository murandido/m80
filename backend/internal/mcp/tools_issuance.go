package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/inventory"
)

// defaultSearchDays bounds a search without a query or dates. GET /issuances has no paging,
// and its whole history would be thousands of rows the agent pays for in tokens.
const defaultSearchDays = 7

type searchIssuancesInput struct {
	Query            string `json:"query,omitempty" jsonschema:"Busca por localizador, passageiro, origem ou destino. Quando presente, as datas são ignoradas."`
	DateFrom         string `json:"date_from,omitempty" jsonschema:"Data inicial da emissão (AAAA-MM-DD), inclusiva. Sem query e sem datas: últimos 7 dias."`
	DateTo           string `json:"date_to,omitempty" jsonschema:"Data final da emissão (AAAA-MM-DD), inclusiva."`
	LoyaltyProgramID string `json:"loyalty_program_id,omitempty" jsonschema:"Filtra por programa (uuid). Veja list_loyalty_programs."`
	AccountID        string `json:"account_id,omitempty" jsonschema:"Filtra por conta (uuid)."`
	AgencyID         string `json:"agency_id,omitempty" jsonschema:"Filtra por agência (uuid)."`
	Status           string `json:"status,omitempty" jsonschema:"Filtra pelo status: incomplete, complete ou cancelled."`
}

// issuanceSummary is one row of a search. The full issuance (with its account, flights,
// passengers and adjustments) is what get_issuance is for; a list of those would cost the
// agent thousands of tokens per row it never reads.
type issuanceSummary struct {
	ID                 string  `json:"id"`
	Status             string  `json:"status"`
	IssuedOn           string  `json:"issued_on"`
	Program            string  `json:"program"`
	Airline            string  `json:"airline"`
	ProgramLocator     string  `json:"program_locator"`
	AirlineLocator     *string `json:"airline_locator"`
	MainPassengerName  *string `json:"main_passenger_name"`
	Origin             *string `json:"origin"`
	Destination        *string `json:"destination"`
	FlightDate         *string `json:"flight_date"`
	Points             int     `json:"points"`
	ValueOfTheThousand int     `json:"value_of_the_thousand"`
	Taxes              *int    `json:"taxes"`
	AccountID          string  `json:"account_id"`
	AccountNickname    *string `json:"account_nickname"`
	Agency             string  `json:"agency"`
	Issuer             string  `json:"issuer"`
	Observation        *string `json:"observation"`
	RefundedPoints     *int    `json:"refunded_points,omitempty"`
}

type apiIssuance struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	IssuedOn       string `json:"issued_on"`
	LoyaltyProgram *struct {
		Code string `json:"code"`
	} `json:"loyalty_program"`
	Airline *struct {
		Name string `json:"name"`
	} `json:"airline"`
	ProgramLocator      string  `json:"program_locator"`
	AirlineLocator      *string `json:"airline_locator"`
	MainPassengerName   *string `json:"main_passenger_name"`
	OriginIATACode      *string `json:"origin_iata_code"`
	DestinationIATACode *string `json:"destination_iata_code"`
	Flights             []struct {
		DepartureAt string `json:"departure_at"`
	} `json:"flights"`
	Points             int    `json:"points"`
	ValueOfTheThousand int    `json:"value_of_the_thousand"`
	Taxes              *int   `json:"taxes"`
	AccountID          string `json:"account_id"`
	AccountNickname    *struct {
		Nickname string `json:"nickname"`
	} `json:"account_nickname"`
	Agency *struct {
		Name string `json:"name"`
	} `json:"agency"`
	Issuer *struct {
		Name string `json:"name"`
	} `json:"issuer"`
	Observation    *string `json:"observation"`
	RefundedPoints *int    `json:"refunded_points"`
}

func (i apiIssuance) summary() issuanceSummary {
	out := issuanceSummary{
		ID:                 i.ID,
		Status:             i.Status,
		IssuedOn:           i.IssuedOn,
		ProgramLocator:     i.ProgramLocator,
		AirlineLocator:     i.AirlineLocator,
		MainPassengerName:  i.MainPassengerName,
		Origin:             i.OriginIATACode,
		Destination:        i.DestinationIATACode,
		Points:             i.Points,
		ValueOfTheThousand: i.ValueOfTheThousand,
		Taxes:              i.Taxes,
		AccountID:          i.AccountID,
		Observation:        i.Observation,
		RefundedPoints:     i.RefundedPoints,
	}
	if i.LoyaltyProgram != nil {
		out.Program = i.LoyaltyProgram.Code
	}
	if i.Airline != nil {
		out.Airline = i.Airline.Name
	}
	if len(i.Flights) > 0 && len(i.Flights[0].DepartureAt) >= 10 {
		day := i.Flights[0].DepartureAt[:10]
		out.FlightDate = &day
	}
	if i.AccountNickname != nil {
		out.AccountNickname = &i.AccountNickname.Nickname
	}
	if i.Agency != nil {
		out.Agency = i.Agency.Name
	}
	if i.Issuer != nil {
		out.Issuer = i.Issuer.Name
	}
	return out
}

func (s *Server) searchIssuances(ctx context.Context, who caller, in searchIssuancesInput) ([]issuanceSummary, error) {
	query := url.Values{}
	if in.Query != "" {
		query.Set("q", in.Query)
	} else {
		from, to := in.DateFrom, in.DateTo
		if from == "" && to == "" {
			today := inventory.DayOf(s.now())
			from = today.AddDate(0, 0, -defaultSearchDays).Format("2006-01-02")
		}
		if from != "" {
			query.Set("date_from", from)
		}
		if to != "" {
			query.Set("date_to", to)
		}
		// Every issuer: the list otherwise defaults to the caller's own issuances, and the
		// agent looks after the whole operation, not the token owner's rows.
		query.Set("issuer_id", "all")
	}
	for key, value := range map[string]string{
		"loyalty_program_id": in.LoyaltyProgramID,
		"account_id":         in.AccountID,
		"agency_id":          in.AgencyID,
	} {
		if value != "" {
			query.Set(key, value)
		}
	}

	var issuances []apiIssuance
	if err := s.get(ctx, who, "/issuances", query, &issuances); err != nil {
		return nil, err
	}
	out := make([]issuanceSummary, 0, len(issuances))
	for _, issuance := range issuances {
		if in.Status != "" && issuance.Status != in.Status {
			continue
		}
		out = append(out, issuance.summary())
	}
	return out, nil
}

type issuanceIDInput struct {
	IssuanceID string `json:"issuance_id" jsonschema:"ID (uuid) da emissão. Veja search_issuances."`
}

type lookupInput struct {
	AirlineID           string  `json:"airline_id" jsonschema:"ID (uuid) da cia aérea."`
	ProgramLocator      string  `json:"program_locator" jsonschema:"Localizador no programa."`
	AirlineLocator      *string `json:"airline_locator,omitempty" jsonschema:"Localizador na cia, quando diferente."`
	OriginIATACode      *string `json:"origin_iata_code,omitempty" jsonschema:"Aeroporto de origem (IATA, ex. GRU)."`
	MainPassengerName   *string `json:"main_passenger_name,omitempty" jsonschema:"Nome do passageiro principal."`
	PassengerLastName   *string `json:"passenger_last_name,omitempty" jsonschema:"Sobrenome usado na busca da cia."`
	LatamPurchaseNumber *string `json:"latam_purchase_number,omitempty" jsonschema:"Número da compra LATAM (LA...)."`
}

type createIssuanceInput struct {
	ProgramLocator      string  `json:"program_locator" jsonschema:"Localizador no programa (obrigatório, único entre as ativas)."`
	LoyaltyProgramID    string  `json:"loyalty_program_id" jsonschema:"ID (uuid) do programa. Veja list_loyalty_programs."`
	AccountID           string  `json:"account_id" jsonschema:"ID (uuid) da conta que pagou os pontos. Veja stock_overview."`
	AccountNicknameID   *string `json:"account_nickname_id,omitempty" jsonschema:"ID do apelido da conta usado, quando houver."`
	Points              int     `json:"points" jsonschema:"Pontos gastos, inteiro (50 mil = 50000)."`
	ValueOfTheThousand  int     `json:"value_of_the_thousand" jsonschema:"Preço de venda do milheiro em centavos por mil pontos (R$ 32,50 = 3250)."`
	Taxes               *int    `json:"taxes,omitempty" jsonschema:"Taxas em centavos."`
	AirlineID           *string `json:"airline_id,omitempty" jsonschema:"ID da cia. Vazio usa a cia padrão do programa."`
	AgencyID            *string `json:"agency_id,omitempty" jsonschema:"ID da agência. Vazio usa a padrão do usuário."`
	IssuerID            *string `json:"issuer_id,omitempty" jsonschema:"ID do emissor (usuário). Vazio = dono do token."`
	AirlineLocator      *string `json:"airline_locator,omitempty" jsonschema:"Localizador na cia."`
	LatamPurchaseNumber *string `json:"latam_purchase_number,omitempty" jsonschema:"Número da compra LATAM."`
	MainPassengerName   *string `json:"main_passenger_name,omitempty" jsonschema:"Passageiro principal."`
	PassengerLastName   *string `json:"passenger_last_name,omitempty" jsonschema:"Sobrenome para a busca na cia."`
	TripType            *string `json:"trip_type,omitempty" jsonschema:"one_way, round_trip ou multi_leg."`
	OriginIATACode      *string `json:"origin_iata_code,omitempty" jsonschema:"Origem (IATA)."`
	DestinationIATACode *string `json:"destination_iata_code,omitempty" jsonschema:"Destino (IATA)."`
	Observation         *string `json:"observation,omitempty" jsonschema:"Observação livre."`
	FlightDate          *string `json:"flight_date,omitempty" jsonschema:"Data do voo de ida (AAAA-MM-DD)."`
	IssuedOn            *string `json:"issued_on,omitempty" jsonschema:"Dia da emissão (AAAA-MM-DD) quando não for hoje."`
}

type updateIssuanceInput struct {
	IssuanceID string               `json:"issuance_id" jsonschema:"ID (uuid) da emissão."`
	Changes    updateIssuanceFields `json:"changes" jsonschema:"Só os campos que mudam."`
}

type updateIssuanceFields struct {
	LoyaltyProgramID    *string `json:"loyalty_program_id,omitempty" jsonschema:"ID do programa."`
	AirlineID           *string `json:"airline_id,omitempty" jsonschema:"ID da cia."`
	AccountID           *string `json:"account_id,omitempty" jsonschema:"ID da conta. Mandar junto account_nickname_id quando houver."`
	AccountNicknameID   *string `json:"account_nickname_id,omitempty" jsonschema:"ID do apelido; só é lido junto com account_id."`
	AgencyID            *string `json:"agency_id,omitempty" jsonschema:"ID da agência."`
	IssuerID            *string `json:"issuer_id,omitempty" jsonschema:"ID do emissor."`
	ProgramLocator      *string `json:"program_locator,omitempty" jsonschema:"Localizador no programa."`
	AirlineLocator      *string `json:"airline_locator,omitempty" jsonschema:"Localizador na cia."`
	LatamPurchaseNumber *string `json:"latam_purchase_number,omitempty" jsonschema:"Número da compra LATAM."`
	MainPassengerName   *string `json:"main_passenger_name,omitempty" jsonschema:"Passageiro principal."`
	PassengerLastName   *string `json:"passenger_last_name,omitempty" jsonschema:"Sobrenome para a busca na cia."`
	TripType            *string `json:"trip_type,omitempty" jsonschema:"one_way, round_trip ou multi_leg."`
	OriginIATACode      *string `json:"origin_iata_code,omitempty" jsonschema:"Origem (IATA)."`
	DestinationIATACode *string `json:"destination_iata_code,omitempty" jsonschema:"Destino (IATA)."`
	Points              *int    `json:"points,omitempty" jsonschema:"Pontos, inteiro."`
	ValueOfTheThousand  *int    `json:"value_of_the_thousand,omitempty" jsonschema:"Milheiro em centavos por mil pontos."`
	Taxes               *int    `json:"taxes,omitempty" jsonschema:"Taxas em centavos."`
	Observation         *string `json:"observation,omitempty" jsonschema:"Observação livre."`
	Status              *string `json:"status,omitempty" jsonschema:"incomplete ou complete. Cancelar é propose_cancel_issuance."`
	FlightDate          *string `json:"flight_date,omitempty" jsonschema:"Data do voo de ida (AAAA-MM-DD)."`
	IssuedOn            *string `json:"issued_on,omitempty" jsonschema:"Move a emissão para outro dia (AAAA-MM-DD), mantendo a hora."`
}

type adjustmentFields struct {
	OccurredOn          string `json:"occurred_on" jsonschema:"Dia em que a cia cobrou ou devolveu (AAAA-MM-DD); decide o período de custeio."`
	Points              int    `json:"points" jsonschema:"Pontos, com sinal: positivo = cobrou mais, negativo = devolveu."`
	Taxes               int    `json:"taxes" jsonschema:"Taxas em centavos, com sinal."`
	ValueOfTheThousand  int    `json:"value_of_the_thousand" jsonschema:"Milheiro cobrado do cliente em centavos por mil pontos (>= 0)."`
	TaxesPassedToClient bool   `json:"taxes_passed_to_client" jsonschema:"As taxas vão para a conta do cliente."`
	Reason              string `json:"reason" jsonschema:"Motivo da alteração (obrigatório), ex.: GRU-CNF trocado para GRU-BHZ, 20/11."`
}

type addAdjustmentInput struct {
	IssuanceID string           `json:"issuance_id" jsonschema:"ID (uuid) da emissão."`
	Adjustment adjustmentFields `json:"adjustment" jsonschema:"A alteração inteira."`
}

type updateAdjustmentInput struct {
	IssuanceID   string           `json:"issuance_id" jsonschema:"ID (uuid) da emissão."`
	AdjustmentID string           `json:"adjustment_id" jsonschema:"ID da alteração. Veja get_issuance."`
	Adjustment   adjustmentFields `json:"adjustment" jsonschema:"A alteração inteira (substitui todos os campos)."`
}

func issuancePath(id string, rest ...string) string {
	path := "/issuances/" + url.PathEscape(id)
	for _, part := range rest {
		path += "/" + part
	}
	return path
}

func (s *Server) registerIssuanceTools(server *sdk.Server) {
	addTool(s, server, &sdk.Tool{
		Name: "search_issuances",
		Description: "Busca emissões e devolve um resumo por linha (status, localizadores, passageiro, trecho, pontos, milheiro e taxas em centavos, conta, agência, emissor). " +
			"Use query para localizador/passageiro, ou datas. Sem nada, traz os últimos 7 dias. Para tudo de uma emissão use get_issuance.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in searchIssuancesInput) (any, error) {
		return s.searchIssuances(ctx, who, in)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "incomplete_issuances",
		Description: "Emissões ainda incompletas (faltando dados como trecho, data do voo ou passageiro) num período, para completar com update_issuance ou sync_issuance_traveos. Padrão: últimos 7 dias.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in searchIssuancesInput) (any, error) {
		in.Query = ""
		in.Status = "incomplete"
		return s.searchIssuances(ctx, who, in)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "get_issuance",
		Description: "Uma emissão completa: voos, passageiros (sem documento), alterações, valores de cancelamento e status.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in issuanceIDInput) (any, error) {
		return s.call(ctx, who, http.MethodGet, issuancePath(in.IssuanceID), nil, nil)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "lookup_issuance",
		Description: "Consulta a reserva na cia/Traveos sem gravar nada: data do voo, origem, destino e passageiro. Útil antes de create_issuance.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(true)},
	}, func(ctx context.Context, who caller, in lookupInput) (any, error) {
		return s.call(ctx, who, http.MethodPost, "/issuances/lookup", nil, in)
	})

	addTool(s, server, &sdk.Tool{
		Name: "create_issuance",
		Description: "Registra uma emissão. Grava direto (ou vira proposta, se o token não puder escrever sozinho) e consome o estoque da conta (FIFO). Recusada se o localizador já existir numa emissão ativa, " +
			"se a conta estiver arquivada ou se a data cair num período fechado. Antes, confira que ela não existe (search_issuances).",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, func(ctx context.Context, who caller, in createIssuanceInput) (any, error) {
		return s.writeOrPropose(ctx, who, "create_issuance", writeIntent{
			Operation: Operation{Method: http.MethodPost, Path: "/issuances", Body: bodyOf(in)},
			Summary:   fmt.Sprintf("Criar emissão %s (%d pontos)", in.ProgramLocator, in.Points),
			Preview:   func() (any, error) { return in, nil },
		})
	})

	addTool(s, server, &sdk.Tool{
		Name:        "update_issuance",
		Description: "Corrige campos de uma emissão (pontos, milheiro, taxas, conta, trecho, passageiro, data, status incomplete/complete...). Grava direto. Não cancela: para isso use propose_cancel_issuance.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
	}, func(ctx context.Context, who caller, in updateIssuanceInput) (any, error) {
		if in.Changes.Status != nil && *in.Changes.Status == "cancelled" {
			return nil, fmt.Errorf("cancelar exige aprovação: use propose_cancel_issuance")
		}
		return s.writeOrPropose(ctx, who, "update_issuance", writeIntent{
			Operation: Operation{Method: http.MethodPatch, Path: issuancePath(in.IssuanceID), Body: bodyOf(in.Changes)},
			Summary:   "Editar emissão " + in.IssuanceID,
			Preview: func() (any, error) {
				current, err := s.issuancePreview(ctx, who, in.IssuanceID)
				return map[string]any{"current": current, "changes": in.Changes}, err
			},
		})
	})

	addTool(s, server, &sdk.Tool{
		Name:        "add_adjustment",
		Description: "Registra uma alteração de passagem (remarcação, troca de trecho) numa emissão ativa: pontos e taxas cobrados ou devolvidos. Grava direto e entra no custeio do dia occurred_on.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, func(ctx context.Context, who caller, in addAdjustmentInput) (any, error) {
		return s.writeOrPropose(ctx, who, "add_adjustment", writeIntent{
			Operation: Operation{Method: http.MethodPost, Path: issuancePath(in.IssuanceID, "adjustments"), Body: bodyOf(in.Adjustment)},
			Summary:   fmt.Sprintf("Registrar alteração (%+d pontos) na emissão %s: %s", in.Adjustment.Points, in.IssuanceID, in.Adjustment.Reason),
			Preview: func() (any, error) {
				current, err := s.issuancePreview(ctx, who, in.IssuanceID)
				return map[string]any{"issuance": current, "adjustment": in.Adjustment}, err
			},
		})
	})

	addTool(s, server, &sdk.Tool{
		Name:        "update_adjustment",
		Description: "Corrige uma alteração de passagem, substituindo todos os campos. Grava direto. Excluir é propose_delete_adjustment.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
	}, func(ctx context.Context, who caller, in updateAdjustmentInput) (any, error) {
		return s.writeOrPropose(ctx, who, "update_adjustment", writeIntent{
			Operation: Operation{Method: http.MethodPatch, Path: issuancePath(in.IssuanceID, "adjustments", url.PathEscape(in.AdjustmentID)), Body: bodyOf(in.Adjustment)},
			Summary:   fmt.Sprintf("Corrigir alteração %s da emissão %s: %s", in.AdjustmentID, in.IssuanceID, in.Adjustment.Reason),
			Preview:   func() (any, error) { return in.Adjustment, nil },
		})
	})

	addTool(s, server, &sdk.Tool{
		Name:        "sync_issuance_traveos",
		Description: "Busca de novo na Traveos/cia os dados da reserva (voos, passageiros) e completa a emissão. Grava direto.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(true)},
	}, func(ctx context.Context, who caller, in issuanceIDInput) (any, error) {
		return s.writeOrPropose(ctx, who, "sync_issuance_traveos", writeIntent{
			Operation: Operation{Method: http.MethodPost, Path: issuancePath(in.IssuanceID, "traveos-sync")},
			Summary:   "Completar a emissão " + in.IssuanceID + " com os dados da Traveos",
		})
	})

	addTool(s, server, &sdk.Tool{
		Name:        "search_clients",
		Description: "Busca clientes por nome, telefone ou localizador de uma emissão.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, in struct {
		Query string `json:"query" jsonschema:"Nome, telefone ou localizador."`
	}) (any, error) {
		var clients json.RawMessage
		return clients, s.get(ctx, who, "/clients", url.Values{"q": {in.Query}}, &clients)
	})
}
