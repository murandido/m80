package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	acct "github.com/murandido/m-80-backend/internal/account"
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

// toPersonPickerResponse is the slim projection GET /people?view=picker returns.
func toPersonPickerResponse(person data.Person) schemas.PersonPickerResponse {
	nicknames := make([]schemas.PersonNicknameResponse, 0, len(person.Nicknames))
	for _, nickname := range person.Nicknames {
		nicknames = append(nicknames, toPersonNicknameResponse(nickname))
	}

	return schemas.PersonPickerResponse{
		ID:        person.ID,
		Name:      person.Name,
		Nicknames: nicknames,
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
		search := db.Where(unaccentedLike("people.name"), like).
			Or(unaccentedLike("people.email"), like).
			Or(unaccentedLike("people.phone"), like).
			Or(unaccentedLike("people.city"), like).
			Or(nicknameMatch("person_nicknames", "person_id", "people", false), like)

		// The CPF is no longer a LIKE, and cannot be: the column holds ciphertext with a random
		// nonce, so there is nothing to match a fragment against. What is left is equality over
		// the blind index, which means the whole document - typing five digits used to find
		// somebody and now finds nobody. That is the trade migration 38 made deliberately.
		//
		// Masked or bare, both reach the same fingerprint: the term is normalized first.
		if fingerprint, ok := searchFingerprint(params.Q); ok {
			search = search.Or("people.cpf_fingerprint = ?", fingerprint)
		}

		query = query.Where(search)
	}

	if complete != nil {
		query = query.Where("people.complete = ?", *complete)
	}

	return query.Order(orderClause(params.Order, peopleOrder, peopleOrder["name_asc"]))
}

// searchFingerprint turns a search term into the blind index of a CPF.
//
// It normalizes exactly the way Person.BeforeSave does - leniently, via NormalizeCPFForIndex -
// and that is not a detail: the registry holds documents that fail the check digits, and
// validating here would make those people impossible to find by their own CPF. The index is
// built from what was typed, so the search has to ask the same question.
//
// A fragment still finds nothing, which is the trade migration 38 made: this is equality over
// a digest, so "52998" only matches somebody whose stored document is literally "52998".
func searchFingerprint(term string) (string, bool) {
	normalized, ok := utils.NormalizeCPFForIndex(term)
	if !ok {
		return "", false
	}
	fingerprint, err := utils.Fingerprint(normalized)
	if err != nil {
		// CPF_INDEX_KEY missing or malformed. Searching by name still works, so this degrades
		// rather than failing the request - but it is worth a line in the log.
		slog.Error("cannot fingerprint a search term, CPF search is disabled", "err", err)
		return "", false
	}
	return fingerprint, true
}

// GetPeople handles the GET /people route.
// @Summary      Listar pessoas
// @Description  Retorna todas as pessoas cadastradas no sistema. Sem paginação — é a rota que alimenta os seletores de titular, administrador e passageiro. Para a tela de gestão, use /people/manage.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome, e-mail, telefone, cidade ou apelido. O CPF é buscado por igualdade e exige os 11 dígitos, porque a coluna é cifrada."
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        complete  query  bool  false  "Somente pessoas completas (true) ou incompletas (false)"
// @Param        view  query  string  false  "view=picker devolve só id, nome e apelidos — o que um combobox usa. Reduz a resposta em cerca de 4x."  Enums(picker)
// @Success      200  {object}  schemas.PersonListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar pessoas"
// @Router       /people [get]
func (h *PeopleHandler) GetPeople(w http.ResponseWriter, r *http.Request) {
	var people []data.Person

	query := peopleListQuery(h.DB, parseListQuery(r), parseOptionalBool(r.URL.Query().Get("complete")))
	picker := r.URL.Query().Get("view") == "picker"

	// The picker only ever shows a name and its nicknames, so it selects three columns
	// instead of fourteen. Leaving cpf out of the SELECT also skips Person.AfterFind's
	// decryption for every row - work whose result the combobox would throw away.
	if picker {
		query = query.Select("people.id", "people.name")
	}

	if err := query.Preload("Nicknames").Find(&people).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch people")
		return
	}

	if picker {
		resp := make([]schemas.PersonPickerResponse, 0, len(people))
		for _, person := range people {
			resp = append(resp, toPersonPickerResponse(person))
		}
		utils.SendJSON(w, r, http.StatusOK, "People fetched successfully", resp)
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
// @Param        q      query  string  false  "Busca por nome, e-mail, telefone, cidade ou apelido. O CPF é buscado por igualdade e exige os 11 dígitos, porque a coluna é cifrada."
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
// @Summary      Excluir titular
// @Description  Exclui o titular e todas as contas dele. Conta sem histórico (emissões, inclusive na lixeira; lotes; períodos; extrato; beneficiários) é apagada de verdade; com histórico, é arquivada. O titular é apagado quando nenhuma conta o referencia mais, e arquivado caso contrário — some do painel e os apelidos dele são liberados.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do titular"
// @Success      200  {object}  schemas.PersonRemovalSingleResponse "O que foi apagado e o que foi arquivado"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Titular não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "A pessoa ainda administra contas de outros titulares"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao excluir"
// @Router       /people/{id} [delete]
func (h *PeopleHandler) DeletePerson(w http.ResponseWriter, r *http.Request) {
	h.removePerson(w, r, false)
}

// PreviewDeletePerson handles the GET /people/{id}/delete-preview route.
// @Summary      Prévia da exclusão do titular
// @Description  Diz o que DELETE /people/{id} faria, sem escrever nada.
// @Tags         Pessoas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do titular"
// @Success      200  {object}  schemas.PersonRemovalSingleResponse
// @Failure      404  {object}  schemas.ErrorResponse "Titular não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "A pessoa ainda administra contas de outros titulares"
// @Router       /people/{id}/delete-preview [get]
func (h *PeopleHandler) PreviewDeletePerson(w http.ResponseWriter, r *http.Request) {
	h.removePerson(w, r, true)
}

func (h *PeopleHandler) removePerson(w http.ResponseWriter, r *http.Request, dryRun bool) {
	id := chi.URLParam(r, "id")

	var removal acct.PersonRemoval
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		removal, err = acct.RemovePerson(tx, id, dryRun)
		return err
	})
	if err != nil {
		switch {
		case acct.IsRemovalRefusal(err):
			utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, gorm.ErrRecordNotFound):
			utils.SendError(w, r, http.StatusNotFound, "Titular não encontrado")
		default:
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete person: "+err.Error())
		}
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Titular excluído", schemas.PersonRemovalResponse{
		AccountsDeleted:  removal.AccountsDeleted,
		AccountsArchived: removal.AccountsArchived,
		PersonDeleted:    removal.PersonDeleted,
	})
}
