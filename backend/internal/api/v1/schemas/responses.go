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

// ErrorResponse is the standard API error response
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Mensagem de erro"`
	Data    any    `json:"data" swaggertype:"object"`
}
