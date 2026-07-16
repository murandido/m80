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

// PersonSingleResponse is the response from single-person routes
type PersonSingleResponse struct {
	Success bool           `json:"success" example:"true"`
	Message string         `json:"message" example:"Pessoa encontrada"`
	Data    PersonResponse `json:"data"`
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

// ErrorResponse is the standard API error response
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Mensagem de erro"`
	Data    any    `json:"data" swaggertype:"object"`
}
