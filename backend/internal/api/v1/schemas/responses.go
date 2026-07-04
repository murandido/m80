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

// ErrorResponse is the standard API error response
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Mensagem de erro"`
	Data    any    `json:"data" swaggertype:"object"`
}
