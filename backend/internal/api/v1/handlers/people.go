package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// postgresForeignKeyViolation is the Postgres error code for a foreign key constraint violation.
const postgresForeignKeyViolation = "23503"

type PeopleHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewPeopleHandler(db *gorm.DB) *PeopleHandler {
	return &PeopleHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toPersonResponse(person data.Person) schemas.PersonResponse {
	nicknames := make([]schemas.PersonNicknameResponse, 0, len(person.Nicknames))
	for _, nickname := range person.Nicknames {
		nicknames = append(nicknames, toPersonNicknameResponse(nickname))
	}

	return schemas.PersonResponse{
		ID:            person.ID,
		Name:          person.Name,
		CPF:           person.CPF,
		BirthDate:     person.BirthDate,
		Email:         person.Email,
		Phone:         person.Phone,
		ZipCode:       person.ZipCode,
		State:         person.State,
		City:          person.City,
		Address:       person.Address,
		AddressNumber: person.AddressNumber,
		Complement:    person.Complement,
		Neighborhood:  person.Neighborhood,
		Nicknames:     nicknames,
		Complete:      person.Complete,
		CreatedAt:     person.CreatedAt,
		UpdatedAt:     person.UpdatedAt,
	}
}

// parseBirthDate parses a "2006-01-02" date string into a *time.Time.
func parseBirthDate(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", *value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// CreatePerson handles the POST /people route
// @Summary      Criar pessoa
// @Description  Cria uma nova pessoa no sistema.
// @Tags         Pessoas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreatePersonRequest true "Dados da nova pessoa"
// @Success      201  {object}  schemas.PersonSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar pessoa"
// @Router       /people [post]
func (h *PeopleHandler) CreatePerson(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreatePersonRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	birthDate, err := parseBirthDate(req.BirthDate)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid birth_date format, expected YYYY-MM-DD")
		return
	}

	person := data.Person{
		Name:          req.Name,
		CPF:           req.CPF,
		BirthDate:     birthDate,
		Email:         req.Email,
		Phone:         req.Phone,
		ZipCode:       req.ZipCode,
		State:         req.State,
		City:          req.City,
		Address:       req.Address,
		AddressNumber: req.AddressNumber,
		Complement:    req.Complement,
		Neighborhood:  req.Neighborhood,
	}

	if req.Complete != nil {
		person.Complete = *req.Complete
	} else {
		person.Complete = person.ComputeComplete()
	}

	if err := h.DB.Create(&person).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create person: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Person created successfully", toPersonResponse(person))
}

// peopleOrder is the whitelist of sort keys; the client's string only ever indexes it.
var peopleOrder = map[string]string{
	"name_asc":     textOrder("people.name", "ASC"),
	"name_desc":    textOrder("people.name", "DESC"),
	"created_desc": "people.created_at DESC",
	"created_asc":  "people.created_at ASC",
}

// peopleListQuery applies the search and ordering shared by /people and /people/manage,
// so the picker and the management screen can never disagree about what a term matches.
func peopleListQuery(db *gorm.DB, params listQuery, complete *bool) *gorm.DB {
	query := db.Model(&data.Person{})

	if params.Q != "" {
		like := likePattern(params.Q)
		query = query.Where(
			db.Where(unaccentedLike("people.name"), like).
				Or(unaccentedLike("people.cpf"), like).
				Or(unaccentedLike("people.email"), like).
				Or(unaccentedLike("people.phone"), like).
				Or(unaccentedLike("people.city"), like).
				Or(nicknameMatch("person_nicknames", "person_id", "people", false), like),
		)
	}

	if complete != nil {
		query = query.Where("people.complete = ?", *complete)
	}

	return query.Order(orderClause(params.Order, peopleOrder, peopleOrder["name_asc"]))
}

// GetPeople handles the GET /people route.
// @Summary      Listar pessoas
// @Description  Retorna todas as pessoas cadastradas no sistema. Sem paginação — é a rota que alimenta os seletores de titular, administrador e passageiro. Para a tela de gestão, use /people/manage.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome, CPF, e-mail, telefone, cidade ou apelido"
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        complete  query  bool  false  "Somente pessoas completas (true) ou incompletas (false)"
// @Success      200  {object}  schemas.PersonListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar pessoas"
// @Router       /people [get]
func (h *PeopleHandler) GetPeople(w http.ResponseWriter, r *http.Request) {
	var people []data.Person

	query := peopleListQuery(h.DB, parseListQuery(r), parseOptionalBool(r.URL.Query().Get("complete")))
	if err := query.Preload("Nicknames").Find(&people).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch people")
		return
	}

	resp := make([]schemas.PersonResponse, 0, len(people))
	for _, person := range people {
		resp = append(resp, toPersonResponse(person))
	}

	utils.SendJSON(w, r, http.StatusOK, "People fetched successfully", resp)
}

// ListPeople handles the GET /people/manage route.
// @Summary      Listar pessoas (paginado)
// @Description  Listagem paginada para a tela de gestão. A rota /people continua devolvendo tudo, porque é ela que alimenta os seletores de titular, administrador e passageiro.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome, CPF, e-mail, telefone, cidade ou apelido"
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        page   query  int     false  "Página (padrão 1)"
// @Param        limit  query  int     false  "Itens por página (padrão 20, máximo 100)"
// @Param        complete  query  bool  false  "Somente pessoas completas (true) ou incompletas (false)"
// @Success      200  {object}  schemas.PersonPaginatedResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar pessoas"
// @Router       /people/manage [get]
func (h *PeopleHandler) ListPeople(w http.ResponseWriter, r *http.Request) {
	params := parseListQuery(r)
	query := peopleListQuery(h.DB, params, parseOptionalBool(r.URL.Query().Get("complete")))

	// The count runs on the same filtered query as the page, so the total the
	// pagination control shows can never disagree with what it is paginating.
	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count people")
		return
	}

	var people []data.Person
	if err := query.Session(&gorm.Session{}).
		Preload("Nicknames").
		Offset((params.Page - 1) * params.Limit).
		Limit(params.Limit).
		Find(&people).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch people")
		return
	}

	items := make([]schemas.PersonResponse, 0, len(people))
	for _, person := range people {
		items = append(items, toPersonResponse(person))
	}

	utils.SendJSON(w, r, http.StatusOK, "People fetched successfully", schemas.PersonPaginatedData{
		People: items,
		Total:  total,
		Page:   params.Page,
		Limit:  params.Limit,
	})
}

// GetPerson handles the GET /people/{id} route
// @Summary      Buscar pessoa por ID
// @Description  Retorna os dados de uma pessoa específica identificada pelo UUID.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da pessoa"
// @Success      200  {object}  schemas.PersonSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa não encontrada"
// @Router       /people/{id} [get]
func (h *PeopleHandler) GetPerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var person data.Person
	if err := h.DB.Preload("Nicknames").First(&person, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Person not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Person fetched successfully", toPersonResponse(person))
}

// UpdatePerson handles the PUT /people/{id} route
// @Summary      Atualizar pessoa
// @Description  Atualiza os dados de uma pessoa existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Pessoas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                      true  "UUID da pessoa"
// @Param        payload body  schemas.UpdatePersonRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.PersonSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Pessoa não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar pessoa"
// @Router       /people/{id} [put]
func (h *PeopleHandler) UpdatePerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var person data.Person
	if err := h.DB.Preload("Nicknames").First(&person, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Person not found")
		return
	}

	var req schemas.UpdatePersonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name != nil {
		person.Name = *req.Name
	}
	if req.CPF != nil {
		person.CPF = req.CPF
	}
	if req.BirthDate != nil {
		birthDate, err := parseBirthDate(req.BirthDate)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Invalid birth_date format, expected YYYY-MM-DD")
			return
		}
		person.BirthDate = birthDate
	}
	if req.Email != nil {
		person.Email = req.Email
	}
	if req.Phone != nil {
		person.Phone = req.Phone
	}
	if req.ZipCode != nil {
		person.ZipCode = req.ZipCode
	}
	if req.State != nil {
		person.State = req.State
	}
	if req.City != nil {
		person.City = req.City
	}
	if req.Address != nil {
		person.Address = req.Address
	}
	if req.AddressNumber != nil {
		person.AddressNumber = req.AddressNumber
	}
	if req.Complement != nil {
		person.Complement = req.Complement
	}
	if req.Neighborhood != nil {
		person.Neighborhood = req.Neighborhood
	}

	if req.Complete != nil {
		person.Complete = *req.Complete
	} else {
		person.Complete = person.ComputeComplete()
	}

	if err := h.DB.Save(&person).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update person: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Person updated successfully", toPersonResponse(person))
}

// DeletePerson handles the DELETE /people/{id} route.
// @Summary      Deletar pessoa
// @Description  Remove uma pessoa do sistema. Falha com 422 se a pessoa estiver vinculada a uma conta.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da pessoa"
// @Success      200  {object}  schemas.ErrorResponse "Pessoa deletada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Pessoa vinculada a uma conta"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar pessoa"
// @Router       /people/{id} [delete]
func (h *PeopleHandler) DeletePerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.Person{}, "id = ?", id).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir pessoa vinculada a uma conta")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete person: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Person deleted successfully", nil)
}
