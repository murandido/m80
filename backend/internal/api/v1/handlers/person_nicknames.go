package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// postgresUniqueViolation is the Postgres error code for a unique constraint violation.
const postgresUniqueViolation = "23505"

type PersonNicknameHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewPersonNicknameHandler(db *gorm.DB) *PersonNicknameHandler {
	return &PersonNicknameHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toPersonNicknameResponse(nickname data.PersonNickname) schemas.PersonNicknameResponse {
	return schemas.PersonNicknameResponse{
		ID:        nickname.ID,
		PersonID:  nickname.PersonID,
		Nickname:  nickname.Nickname,
		CreatedAt: nickname.CreatedAt,
	}
}

// personExists reports whether a person with the given ID exists.
func (h *PersonNicknameHandler) personExists(personID string) bool {
	return h.DB.First(&data.Person{}, "id = ?", personID).Error == nil
}

// CreatePersonNickname handles the POST /people/{id}/nicknames route
// @Summary      Criar apelido de pessoa
// @Description  Cria um novo apelido para a pessoa identificada pelo UUID.
// @Tags         Apelidos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                             true  "UUID da pessoa"
// @Param        payload  body  schemas.CreatePersonNicknameRequest true  "Dados do novo apelido"
// @Success      201  {object}  schemas.PersonNicknameSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Apelido já está em uso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar apelido"
// @Router       /people/{id}/nicknames [post]
func (h *PersonNicknameHandler) CreatePersonNickname(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")

	if !h.personExists(personID) {
		utils.SendError(w, r, http.StatusNotFound, "Person not found")
		return
	}

	var req schemas.CreatePersonNicknameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// The alias reaches every account the person holds too (see internal/account/nicknames.go),
	// all in one transaction so a conflict on any of them leaves nothing half-written.
	var nickname data.PersonNickname
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := acct.ShareNickname(tx, personID, req.Nickname); err != nil {
			return err
		}
		return tx.Where("person_id = ? AND LOWER(nickname) = LOWER(?)", personID, req.Nickname).
			First(&nickname).Error
	})
	if err != nil {
		sendSharedNicknameError(w, r, err)
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Nickname created successfully", toPersonNicknameResponse(nickname))
}

// CheckNicknameAvailability handles the GET /people/nicknames/availability route.
// @Summary      Verificar disponibilidade de apelido
// @Description  Verifica se um apelido já está em uso por alguém no sistema.
// @Tags         Apelidos
// @Produce      json
// @Security     BearerAuth
// @Param        nickname  query  string  true  "Apelido a verificar"
// @Success      200  {object}  schemas.NicknameAvailabilityCheckResponse
// @Failure      400  {object}  schemas.ErrorResponse "Parâmetro nickname ausente"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao verificar disponibilidade"
// @Router       /people/nicknames/availability [get]
func (h *PersonNicknameHandler) CheckNicknameAvailability(w http.ResponseWriter, r *http.Request) {
	nickname := r.URL.Query().Get("nickname")
	if nickname == "" {
		utils.SendError(w, r, http.StatusBadRequest, "nickname query param is required")
		return
	}

	var count int64
	if err := h.DB.Model(&data.PersonNickname{}).Where("nickname = ?", nickname).Count(&count).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to check nickname availability")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Disponibilidade verificada", schemas.NicknameAvailabilityResponse{Available: count == 0})
}

// GetPersonNicknames handles the GET /people/{id}/nicknames route.
// @Summary      Listar apelidos de uma pessoa
// @Description  Retorna todos os apelidos cadastrados para a pessoa identificada pelo UUID.
// @Tags         Apelidos
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da pessoa"
// @Success      200  {object}  schemas.PersonNicknameListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar apelidos"
// @Router       /people/{id}/nicknames [get]
func (h *PersonNicknameHandler) GetPersonNicknames(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")

	if !h.personExists(personID) {
		utils.SendError(w, r, http.StatusNotFound, "Person not found")
		return
	}

	var nicknames []data.PersonNickname
	if err := h.DB.Where("person_id = ?", personID).Find(&nicknames).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch nicknames")
		return
	}

	resp := make([]schemas.PersonNicknameResponse, 0, len(nicknames))
	for _, nickname := range nicknames {
		resp = append(resp, toPersonNicknameResponse(nickname))
	}

	utils.SendJSON(w, r, http.StatusOK, "Nicknames fetched successfully", resp)
}

// GetPersonNickname handles the GET /people/{id}/nicknames/{nicknameID} route
// @Summary      Buscar apelido por ID
// @Description  Retorna os dados de um apelido específico de uma pessoa.
// @Tags         Apelidos
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string  true  "UUID da pessoa"
// @Param        nicknameID  path  string  true  "UUID do apelido"
// @Success      200  {object}  schemas.PersonNicknameSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Router       /people/{id}/nicknames/{nicknameID} [get]
func (h *PersonNicknameHandler) GetPersonNickname(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.PersonNickname
	if err := h.DB.First(&nickname, "id = ? AND person_id = ?", nicknameID, personID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname fetched successfully", toPersonNicknameResponse(nickname))
}

// UpdatePersonNickname handles the PUT /people/{id}/nicknames/{nicknameID} route
// @Summary      Atualizar apelido
// @Description  Atualiza o apelido de uma pessoa existente.
// @Tags         Apelidos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string                             true  "UUID da pessoa"
// @Param        nicknameID  path  string                             true  "UUID do apelido"
// @Param        payload     body  schemas.UpdatePersonNicknameRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.PersonNicknameSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Apelido já está em uso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar apelido"
// @Router       /people/{id}/nicknames/{nicknameID} [put]
func (h *PersonNicknameHandler) UpdatePersonNickname(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.PersonNickname
	if err := h.DB.First(&nickname, "id = ? AND person_id = ?", nicknameID, personID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	var req schemas.UpdatePersonNicknameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Nickname != nil {
		nickname.Nickname = *req.Nickname
	}

	if err := h.DB.Save(&nickname).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Apelido já está em uso")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname updated successfully", toPersonNicknameResponse(nickname))
}

// DeletePersonNickname handles the DELETE /people/{id}/nicknames/{nicknameID} route.
// @Summary      Deletar apelido
// @Description  Remove um apelido de uma pessoa.
// @Tags         Apelidos
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string  true  "UUID da pessoa"
// @Param        nicknameID  path  string  true  "UUID do apelido"
// @Success      200  {object}  schemas.ErrorResponse "Apelido deletado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar apelido"
// @Router       /people/{id}/nicknames/{nicknameID} [delete]
func (h *PersonNicknameHandler) DeletePersonNickname(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.PersonNickname
	if err := h.DB.First(&nickname, "id = ? AND person_id = ?", nicknameID, personID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	// Leaves the person's accounts as well: the alias is one thing across the holder's row.
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		return acct.UnshareNickname(tx, personID, nickname.Nickname)
	}); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname deleted successfully", nil)
}

// sendSharedNicknameError turns a conflict on any side of a shared alias into the 422 the
// panel shows next to the field, naming where the alias is already in use.
func sendSharedNicknameError(w http.ResponseWriter, r *http.Request, err error) {
	var taken *acct.NicknameTakenError
	if errors.As(err, &taken) {
		utils.SendError(w, r, http.StatusUnprocessableEntity, taken.Error())
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Apelido já está em uso")
		return
	}
	utils.SendError(w, r, http.StatusInternalServerError, "Failed to save nickname: "+err.Error())
}
