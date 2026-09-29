package schemas

// AuthLoginData is the data payload returned upon login
type AuthLoginData struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// AuthLoginResponse is the POST /auth/login response route
type AuthLoginResponse struct {
	Success bool          `json:"success" example:"true"`
	Message string        `json:"message" example:"Login realizado com sucesso"`
	Data    AuthLoginData `json:"data"`
}

// AuthMeResponse is the GET /auth/me response route
type AuthMeResponse struct {
	Success bool         `json:"success" example:"true"`
	Message string       `json:"message" example:"Dados recuperados com sucesso"`
	Data    UserResponse `json:"data"`
}

// UserListResponse is the GET /users route response
type UserListResponse struct {
	Success bool           `json:"success" example:"true"`
	Message string         `json:"message" example:"Usuários listados com sucesso"`
	Data    []UserResponse `json:"data"`
}

// UserSingleResponse is the response from single-user routes
type UserSingleResponse struct {
	Success bool         `json:"success" example:"true"`
	Message string       `json:"message" example:"Usuário encontrado"`
	Data    UserResponse `json:"data"`
}

// PersonListResponse is the GET /people route response
type PersonListResponse struct {
	Success bool             `json:"success" example:"true"`
	Message string           `json:"message" example:"Pessoas listadas com sucesso"`
	Data    []PersonResponse `json:"data"`
}

// PersonPaginatedData is the data payload for the paginated people listing.
type PersonPaginatedData struct {
	People []PersonResponse `json:"people"`
	Total  int64            `json:"total"`
	Page   int              `json:"page"`
	Limit  int              `json:"limit"`
}

// PersonPaginatedResponse is the GET /people/manage route response
type PersonPaginatedResponse struct {
	Success bool                `json:"success" example:"true"`
	Message string              `json:"message" example:"Pessoas listadas com sucesso"`
	Data    PersonPaginatedData `json:"data"`
}

// PersonSingleResponse is the response from single-person routes
type PersonSingleResponse struct {
	Success bool           `json:"success" example:"true"`
	Message string         `json:"message" example:"Pessoa encontrada"`
	Data    PersonResponse `json:"data"`
}

// MergePersonPreviewSingleResponse is the GET /people/{id}/merge-preview route response
type MergePersonPreviewSingleResponse struct {
	Success bool                       `json:"success" example:"true"`
	Message string                     `json:"message" example:"Prévia calculada com sucesso"`
	Data    MergePersonPreviewResponse `json:"data"`
}

// MergePersonResultSingleResponse is the POST /people/{id}/merge route response
type MergePersonResultSingleResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Cadastros mesclados com sucesso"`
	Data    MergePersonResultResponse `json:"data"`
}

// MergeRowPreviewSingleResponse is the GET /people/{id}/merge-row-preview route response
type MergeRowPreviewSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Prévia calculada com sucesso"`
	Data    MergeRowPreviewResponse `json:"data"`
}

// MergeRowResultSingleResponse is the POST /people/{id}/merge-row route response
type MergeRowResultSingleResponse struct {
	Success bool                   `json:"success" example:"true"`
	Message string                 `json:"message" example:"Contas mescladas com sucesso"`
	Data    MergeRowResultResponse `json:"data"`
}

// PersonNicknameListResponse is the GET /people/{id}/nicknames route response
type PersonNicknameListResponse struct {
	Success bool                     `json:"success" example:"true"`
	Message string                   `json:"message" example:"Apelidos listados com sucesso"`
	Data    []PersonNicknameResponse `json:"data"`
}

// PersonNicknameSingleResponse is the response from single-person-nickname routes
type PersonNicknameSingleResponse struct {
	Success bool                   `json:"success" example:"true"`
	Message string                 `json:"message" example:"Apelido encontrado"`
	Data    PersonNicknameResponse `json:"data"`
}

// NicknameAvailabilityCheckResponse is the GET /people/nicknames/availability route response
type NicknameAvailabilityCheckResponse struct {
	Success bool                         `json:"success" example:"true"`
	Message string                       `json:"message" example:"Disponibilidade verificada"`
	Data    NicknameAvailabilityResponse `json:"data"`
}

// AccountListResponse is the GET /accounts route response
type AccountListResponse struct {
	Success bool              `json:"success" example:"true"`
	Message string            `json:"message" example:"Contas listadas com sucesso"`
	Data    []AccountResponse `json:"data"`
}

// AccountPaginatedData is the data payload for the paginated account listing.
type AccountPaginatedData struct {
	Accounts []AccountResponse `json:"accounts"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	Limit    int               `json:"limit"`
}

// AccountPaginatedResponse is the GET /accounts/manage route response
type AccountPaginatedResponse struct {
	Success bool                 `json:"success" example:"true"`
	Message string               `json:"message" example:"Contas listadas com sucesso"`
	Data    AccountPaginatedData `json:"data"`
}

// AccountHolderRow is one row of the accounts screen: a holder and every loyalty account
// they hold. Holder is null on a row that is a single account with no holder.
type AccountHolderRow struct {
	Holder   *PersonResponse   `json:"holder"`
	Accounts []AccountResponse `json:"accounts"`
}

// AccountHolderPaginatedData is the data payload for the listing grouped by holder.
type AccountHolderPaginatedData struct {
	Rows  []AccountHolderRow `json:"rows"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

// AccountHolderPaginatedResponse is the GET /accounts/holders route response
type AccountHolderPaginatedResponse struct {
	Success bool                       `json:"success" example:"true"`
	Message string                     `json:"message" example:"Contas listadas com sucesso"`
	Data    AccountHolderPaginatedData `json:"data"`
}

// AccountSingleResponse is the response from single-account routes
type AccountSingleResponse struct {
	Success bool            `json:"success" example:"true"`
	Message string          `json:"message" example:"Conta encontrada"`
	Data    AccountResponse `json:"data"`
}

// AccountNicknameListResponse is the GET /accounts/{id}/nicknames route response
type AccountNicknameListResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Apelidos listados com sucesso"`
	Data    []AccountNicknameResponse `json:"data"`
}

// AccountNicknameSingleResponse is the response from single-account-nickname routes
type AccountNicknameSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Apelido encontrado"`
	Data    AccountNicknameResponse `json:"data"`
}

// AirlineListResponse is the GET /airlines route response
type AirlineListResponse struct {
	Success bool              `json:"success" example:"true"`
	Message string            `json:"message" example:"Companhias aéreas listadas com sucesso"`
	Data    []AirlineResponse `json:"data"`
}

// AirlineSingleResponse is the response from single-airline routes
type AirlineSingleResponse struct {
	Success bool            `json:"success" example:"true"`
	Message string          `json:"message" example:"Companhia aérea encontrada"`
	Data    AirlineResponse `json:"data"`
}

// AirportListResponse is the GET /airports route response
type AirportListResponse struct {
	Success bool              `json:"success" example:"true"`
	Message string            `json:"message" example:"Aeroportos listados com sucesso"`
	Data    []AirportResponse `json:"data"`
}

// AirportPaginatedResponse is the GET /airports/manage route response
type AirportPaginatedResponse struct {
	Success bool                 `json:"success" example:"true"`
	Message string               `json:"message" example:"Aeroportos listados com sucesso"`
	Data    AirportPaginatedData `json:"data"`
}

// AirportSingleResponse is the response from single-airport routes
type AirportSingleResponse struct {
	Success bool            `json:"success" example:"true"`
	Message string          `json:"message" example:"Aeroporto encontrado"`
	Data    AirportResponse `json:"data"`
}

// ObservationPresetListResponse is the GET /observation-presets route response
type ObservationPresetListResponse struct {
	Success bool                        `json:"success" example:"true"`
	Message string                      `json:"message" example:"Observações listadas com sucesso"`
	Data    []ObservationPresetResponse `json:"data"`
}

// ObservationPresetSingleResponse is the response from single-observation-preset routes
type ObservationPresetSingleResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Observação encontrada"`
	Data    ObservationPresetResponse `json:"data"`
}

// LoyaltyProgramListResponse is the GET /loyalty-programs route response
type LoyaltyProgramListResponse struct {
	Success bool                     `json:"success" example:"true"`
	Message string                   `json:"message" example:"Programas de fidelidade listados com sucesso"`
	Data    []LoyaltyProgramResponse `json:"data"`
}

// LoyaltyProgramSingleResponse is the response from single-loyalty-program routes
type LoyaltyProgramSingleResponse struct {
	Success bool                   `json:"success" example:"true"`
	Message string                 `json:"message" example:"Programa de fidelidade encontrado"`
	Data    LoyaltyProgramResponse `json:"data"`
}

// AgencyListResponse is the GET /agencies route response
type AgencyListResponse struct {
	Success bool             `json:"success" example:"true"`
	Message string           `json:"message" example:"Agências listadas com sucesso"`
	Data    []AgencyResponse `json:"data"`
}

// AgencySingleResponse is the response from single-agency routes
type AgencySingleResponse struct {
	Success bool           `json:"success" example:"true"`
	Message string         `json:"message" example:"Agência encontrada"`
	Data    AgencyResponse `json:"data"`
}

// ClientListResponse is the GET /clients route response
type ClientListResponse struct {
	Success bool             `json:"success" example:"true"`
	Message string           `json:"message" example:"Clientes listados com sucesso"`
	Data    []ClientResponse `json:"data"`
}

// ClientPaginatedData is the data payload for the paginated client listing.
type ClientPaginatedData struct {
	Clients []ClientResponse `json:"clients"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	Limit   int              `json:"limit"`
}

// ClientPaginatedResponse is the GET /clients/manage route response
type ClientPaginatedResponse struct {
	Success bool                `json:"success" example:"true"`
	Message string              `json:"message" example:"Clientes listados com sucesso"`
	Data    ClientPaginatedData `json:"data"`
}

// ClientSingleResponse is the response from single-client routes
type ClientSingleResponse struct {
	Success bool           `json:"success" example:"true"`
	Message string         `json:"message" example:"Cliente encontrado"`
	Data    ClientResponse `json:"data"`
}

// IssuanceListResponse is the GET /issuances route response
type IssuanceListResponse struct {
	Success bool               `json:"success" example:"true"`
	Message string             `json:"message" example:"Emissões listadas com sucesso"`
	Data    []IssuanceResponse `json:"data"`
}

// IssuanceSingleResponse is the response from single-issuance routes
type IssuanceSingleResponse struct {
	Success bool             `json:"success" example:"true"`
	Message string           `json:"message" example:"Emissão encontrada"`
	Data    IssuanceResponse `json:"data"`
}

// IssuanceSummaryListResponse is the GET /issuances/summary route response
type IssuanceSummaryListResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Emissões listadas com sucesso"`
	Data    []IssuanceSummaryResponse `json:"data"`
}

// IssuancePassengerListResponse is the GET /issuances/{id}/passengers route response
type IssuancePassengerListResponse struct {
	Success bool                        `json:"success" example:"true"`
	Message string                      `json:"message" example:"Passageiros listados com sucesso"`
	Data    []IssuancePassengerResponse `json:"data"`
}

// IssuancePassengerSingleResponse is the response from single-issuance-passenger routes
type IssuancePassengerSingleResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Passageiro encontrado"`
	Data    IssuancePassengerResponse `json:"data"`
}

// SaleListResponse is the GET /sales route response
type SaleListResponse struct {
	Success bool           `json:"success" example:"true"`
	Message string         `json:"message" example:"Vendas listadas com sucesso"`
	Data    []SaleResponse `json:"data"`
}

// SaleSingleResponse is the response from single-sale routes
type SaleSingleResponse struct {
	Success bool         `json:"success" example:"true"`
	Message string       `json:"message" example:"Venda atualizada com sucesso"`
	Data    SaleResponse `json:"data"`
}

// SalePaymentListResponse is the GET /sales/{id}/payments route response
type SalePaymentListResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Lançamentos listados com sucesso"`
	Data    []SalePaymentResponse `json:"data"`
}

// SalePaymentSingleResponse is the POST /sales/{id}/payments route response
type SalePaymentSingleResponse struct {
	Success bool                `json:"success" example:"true"`
	Message string              `json:"message" example:"Lançamento registrado com sucesso"`
	Data    SalePaymentResponse `json:"data"`
}

// SupplierBalanceListResponse is the GET /sales/suppliers route response
type SupplierBalanceListResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Saldos por fornecedor calculados com sucesso"`
	Data    []SupplierBalanceResponse `json:"data"`
}

// UpdateSupplierIssuancesSingleResponse is the POST /sales/suppliers/issuances route response
type UpdateSupplierIssuancesSingleResponse struct {
	Success bool                            `json:"success" example:"true"`
	Message string                          `json:"message" example:"Emissões atualizadas com sucesso"`
	Data    UpdateSupplierIssuancesResponse `json:"data"`
}

// SupplierExportCounterSingleResponse is the response of the supplier export-counter routes
type SupplierExportCounterSingleResponse struct {
	Success bool                          `json:"success" example:"true"`
	Message string                        `json:"message" example:"Contador atualizado com sucesso"`
	Data    SupplierExportCounterResponse `json:"data"`
}

// BulkSupplierExportCounterSingleResponse is the response of the bulk export-counter route
type BulkSupplierExportCounterSingleResponse struct {
	Success bool                              `json:"success" example:"true"`
	Message string                            `json:"message" example:"Contadores atualizados com sucesso"`
	Data    BulkSupplierExportCounterResponse `json:"data"`
}

// DashboardSingleResponse is the GET /dashboard route response
type DashboardSingleResponse struct {
	Success bool              `json:"success" example:"true"`
	Message string            `json:"message" example:"Dashboard calculado com sucesso"`
	Data    DashboardResponse `json:"data"`
}

// ErrorResponse is the standard API error response
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Mensagem de erro"`
	Data    any    `json:"data" swaggertype:"object"`
}

// MergeAccountPreviewSingleResponse is the GET /accounts/{id}/merge-preview route response
type MergeAccountPreviewSingleResponse struct {
	Success bool                        `json:"success" example:"true"`
	Message string                      `json:"message" example:"Prévia calculada com sucesso"`
	Data    MergeAccountPreviewResponse `json:"data"`
}

// PromoRecipeListResponse is the GET /promo-recipes route response
type PromoRecipeListResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Receitas listadas com sucesso"`
	Data    []PromoRecipeResponse `json:"data"`
}

// PromoRecipeSingleResponse is the response from single-recipe routes
type PromoRecipeSingleResponse struct {
	Success bool                `json:"success" example:"true"`
	Message string              `json:"message" example:"Receita criada com sucesso"`
	Data    PromoRecipeResponse `json:"data"`
}

// PromoRecipeTestSingleResponse is the POST /promo-recipes/test route response
type PromoRecipeTestSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Formulário localizado"`
	Data    PromoRecipeTestResponse `json:"data"`
}

// LoyaltyDocumentListResponse is the GET /loyalty-documents route response
type LoyaltyDocumentListResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"CPFs listados com sucesso"`
	Data    []LoyaltyDocumentResponse `json:"data"`
}

// LoyaltyDocumentPaginatedData is the data payload for the paginated document listing.
type LoyaltyDocumentPaginatedData struct {
	Documents []LoyaltyDocumentResponse `json:"documents"`
	Total     int64                     `json:"total"`
	Page      int                       `json:"page"`
	Limit     int                       `json:"limit"`
}

// LoyaltyDocumentPaginatedResponse is the GET /loyalty-documents/manage route response
type LoyaltyDocumentPaginatedResponse struct {
	Success bool                         `json:"success" example:"true"`
	Message string                       `json:"message" example:"CPFs listados com sucesso"`
	Data    LoyaltyDocumentPaginatedData `json:"data"`
}

// LoyaltyDocumentSingleResponse is the response from single-document routes
type LoyaltyDocumentSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"CPF cadastrado com sucesso"`
	Data    LoyaltyDocumentResponse `json:"data"`
}

// PromoEnrollmentListResponse is the GET /promo-enrollments route response
type PromoEnrollmentListResponse struct {
	Success bool                         `json:"success" example:"true"`
	Message string                       `json:"message" example:"Histórico listado com sucesso"`
	Data    PromoEnrollmentPaginatedData `json:"data"`
}

// ClientPaymentHistorySingleResponse is the GET /sales/client-payments route response
type ClientPaymentHistorySingleResponse struct {
	Success bool                         `json:"success" example:"true"`
	Message string                       `json:"message" example:"Pagamentos do cliente listados com sucesso"`
	Data    ClientPaymentHistoryResponse `json:"data"`
}

// ClientPaymentUndoSingleResponse is the DELETE /sales/client-payments/{id} route response
type ClientPaymentUndoSingleResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Pagamento desfeito"`
	Data    ClientPaymentUndoResponse `json:"data"`
}
