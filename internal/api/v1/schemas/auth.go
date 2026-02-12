package schemas

type LoginRequest struct {
	Phone    string `json:"phone" validate:"required,len=11"`
	Password string `json:"password" validate:"required,min=6"`
}

type LoginResponse struct {
	Token string `json:"token"`
}
