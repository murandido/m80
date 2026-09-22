package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type AccountHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAccountHandler(db *gorm.DB) *AccountHandler {
	return &AccountHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

// archivedAt unwraps a gorm.DeletedAt for the wire. Shared by the account and the
// loyalty program responses, which both carry it so the panel can tell an archived
// record apart from a live one when it meets it embedded in a past issuance.
func archivedAt(deletedAt gorm.DeletedAt) *time.Time {
	if !deletedAt.Valid {
		return nil
	}
	return &deletedAt.Time
}

// toAccountResponse renders an account without its CPF consumption. Most callers do not
// have it: computing it costs a query, and the create/update round-trips do not need it.
// balanceDateLayout is the wire format of the day a balance was read: a plain date, like
// every other date the panel types.
const balanceDateLayout = "2006-01-02"

// fillExpectedBalances writes the derived balance onto the responses that have one.
//
// Done here rather than inside the response builder because it is a query, and one for the
// whole page rather than one per row. Accounts that do not own their point inventory come
// back absent from the map and keep a null balance, which is the honest answer for them.
//
// Best effort on purpose: a failure to compute leaves the field null instead of failing the
// listing, because an account screen that will not load over a derived number is worse than
// one missing it.
func fillExpectedBalances(db *gorm.DB, responses []schemas.AccountResponse) {
	if len(responses) == 0 {
		return
	}

	ids := make([]string, 0, len(responses))
	for _, response := range responses {
		if response.OwnsPointInventory {
			ids = append(ids, response.ID)
		}
	}
	if len(ids) == 0 {
		return
	}

	balances, err := inventory.ExpectedBalances(db, ids)
	if err != nil {
		slog.Error("could not compute the expected point balances", "err", err)
		return
	}
	for i := range responses {
		if points, ok := balances[responses[i].ID]; ok {
			expected := points
			responses[i].ExpectedBalancePoints = &expected
		}
	}
}

// fillLastSynced writes each connected account's last successful sync time onto its
// response, for the card's "atualizado há…". One query per programme for the whole page, like
// fillExpectedBalances, and best-effort for the same reason: a failed read leaves the field null
// rather than failing the listing. The Connected bits are already set by the response builder.
func fillLastSynced(db *gorm.DB, responses []schemas.AccountResponse) {
	if len(responses) == 0 {
		return
	}

	azulIDs := make([]string, 0, len(responses))
	smilesIDs := make([]string, 0, len(responses))
	for _, response := range responses {
		if response.AzulConnected {
			azulIDs = append(azulIDs, response.ID)
		}
		if response.SmilesConnected {
			smilesIDs = append(smilesIDs, response.ID)
		}
	}

	if len(azulIDs) > 0 {
		var states []data.AzulSyncState
		if err := db.Where("account_id IN ? AND last_synced_at IS NOT NULL", azulIDs).
			Find(&states).Error; err != nil {
			slog.Error("could not load the azul sync states", "err", err)
		} else {
			lastSynced := make(map[string]time.Time, len(states))
			for _, state := range states {
				if state.LastSyncedAt != nil {
					lastSynced[state.AccountID] = *state.LastSyncedAt
				}
			}
			for i := range responses {
				if at, ok := lastSynced[responses[i].ID]; ok {
					synced := at
					responses[i].AzulLastSyncedAt = &synced
				}
			}
		}
	}

	if len(smilesIDs) == 0 {
		return
	}
	var smilesStates []data.SmilesSyncState
	if err := db.Where("account_id IN ? AND last_synced_at IS NOT NULL", smilesIDs).
		Find(&smilesStates).Error; err != nil {
		slog.Error("could not load the smiles sync states", "err", err)
		return
	}
	smilesSynced := make(map[string]time.Time, len(smilesStates))
	for _, state := range smilesStates {
		if state.LastSyncedAt != nil {
			smilesSynced[state.AccountID] = *state.LastSyncedAt
		}
	}
	for i := range responses {
		if at, ok := smilesSynced[responses[i].ID]; ok {
			synced := at
			responses[i].SmilesLastSyncedAt = &synced
		}
	}
}

func toAccountResponse(account data.Account) schemas.AccountResponse {
	return toAccountResponseWithUsage(account, nil)
}

func toAccountResponseWithUsage(account data.Account, usage *acct.Usage) schemas.AccountResponse {
	nicknames := make([]schemas.AccountNicknameResponse, 0, len(account.Nicknames))
	for _, nickname := range account.Nicknames {
		nicknames = append(nicknames, toAccountNicknameResponse(nickname))
	}

	var loyaltyProgram schemas.LoyaltyProgramResponse
	if account.LoyaltyProgram != nil {
		loyaltyProgram = schemas.LoyaltyProgramResponse{
			ID:               account.LoyaltyProgram.ID,
			Name:             account.LoyaltyProgram.Name,
			Code:             account.LoyaltyProgram.Code,
			Color:            account.LoyaltyProgram.Color,
			DefaultAirlineID: account.LoyaltyProgram.DefaultAirlineID,
			CreatedAt:        account.LoyaltyProgram.CreatedAt,
			UpdatedAt:        account.LoyaltyProgram.UpdatedAt,
			DeletedAt:        archivedAt(account.LoyaltyProgram.DeletedAt),
		}
	}

	var checkedOn *string
	if account.RealBalanceCheckedOn != nil {
		day := account.RealBalanceCheckedOn.Format(balanceDateLayout)
		checkedOn = &day
	}

	return schemas.AccountResponse{
		ID:                        account.ID,
		Category:                  account.Category,
		AzulConnected:             account.AzulCustomerKey != nil && *account.AzulCustomerKey != "",
		SmilesConnected:           account.SmilesRefreshToken != nil && *account.SmilesRefreshToken != "",
		RealBalancePoints:         account.RealBalancePoints,
		RealBalanceCheckedOn:      checkedOn,
		WaivesRedemptionFee:       account.WaivesRedemptionFee,
		OwnsPointInventory:        account.OwnsPointInventory,
		TracksStatement:           account.TracksStatement,
		DefaultProfitSharePercent: account.DefaultProfitSharePercent,
		CPFUsage:                  usage,
		LoyaltyProgramID:          account.LoyaltyProgramID,
		LoyaltyProgram:            loyaltyProgram,
		Login:                     account.Login,
		Password:                  account.Password,
		HolderID:                  account.HolderID,
		AdministratorID:           account.AdministratorID,
		Nicknames:                 nicknames,
		Complete:                  account.Complete,
		CreatedAt:                 account.CreatedAt,
		UpdatedAt:                 account.UpdatedAt,
		DeletedAt:                 archivedAt(account.DeletedAt),
	}
}

// toAccountPickerResponse projects an account down to what a combobox draws. It builds
// the response directly rather than trimming a full AccountResponse, so adding a field
// to the wide shape does not silently widen the picker feed as well.
func toAccountPickerResponse(account data.Account) schemas.AccountPickerResponse {
	nicknames := make([]schemas.AccountNicknameOption, 0, len(account.Nicknames))
	for _, nickname := range account.Nicknames {
		nicknames = append(nicknames, schemas.AccountNicknameOption{
			ID:       nickname.ID,
			Nickname: nickname.Nickname,
		})
	}

	return schemas.AccountPickerResponse{
		ID:                  account.ID,
		LoyaltyProgramID:    account.LoyaltyProgramID,
		Login:               account.Login,
		HolderID:            account.HolderID,
		Nicknames:           nicknames,
		WaivesRedemptionFee: account.WaivesRedemptionFee,
		OwnsPointInventory:  account.OwnsPointInventory,
		TracksStatement:     account.TracksStatement,
		DeletedAt:           archivedAt(account.DeletedAt),
	}
}

// CreateAccount handles the POST /accounts route
// @Summary      Criar conta
// @Description  Cria uma nova conta de programa de fidelidade no sistema.
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateAccountRequest true "Dados da nova conta"
// @Success      201  {object}  schemas.AccountSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Programa de fidelidade, titular ou administrador informado não existe"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar conta"
// @Router       /accounts [post]
func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateAccountRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	account := data.Account{
		LoyaltyProgramID: req.LoyaltyProgramID,
		Login:            req.Login,
		Password:         req.Password,
		HolderID:         req.HolderID,
		AdministratorID:  req.AdministratorID,
	}
	if req.WaivesRedemptionFee != nil {
		account.WaivesRedemptionFee = *req.WaivesRedemptionFee
	}
	if req.OwnsPointInventory != nil {
		account.OwnsPointInventory = *req.OwnsPointInventory
	}
	if req.TracksStatement != nil {
		account.TracksStatement = *req.TracksStatement
	}
	if req.DefaultProfitSharePercent != nil {
		account.DefaultProfitSharePercent = *req.DefaultProfitSharePercent
	}
	if req.Category != nil {
		account.Category = *req.Category
	}

	if req.Complete != nil {
		account.Complete = *req.Complete
	} else {
		account.Complete = account.ComputeComplete()
	}

	if err := h.DB.Create(&account).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Programa de fidelidade, titular ou administrador informado não existe")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create account: "+err.Error())
		return
	}

	// Reload: BeforeSave leaves Password encrypted in memory, and only a fresh
	// read triggers AfterFind to decrypt it back for the response.
	if err := h.DB.Preload("LoyaltyProgram").First(&account, "id = ?", account.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load created account: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Account created successfully", toAccountResponse(account))
}

// An account has no name of its own - the panel labels it by holder, then login, then
// alias (see account-card.tsx). "Alphabetical" therefore has to be an expression
// rather than a column, and it has to follow the same chain the operator reads: sort
// by holder and login alone and an account named by its alias lands in the NULL
// bucket at the end, which just looks like the list is not sorted.
//
// The alias comes from a sub-select for the same reason the suppliers view uses one:
// an account can hold several, and joining would duplicate the row. Oldest wins, so
// the position does not move when someone registers a second alias.
//
// Accounts with none of the three still sort last rather than jumping to the top on
// an empty string, and created_at breaks the ties so the order is stable.
const accountLabelOrder = `COALESCE(
	NULLIF(holders.name, ''),
	NULLIF(accounts.login, ''),
	(SELECT nickname FROM account_nicknames
		WHERE account_nicknames.account_id = accounts.id
		  AND account_nicknames.deleted_at IS NULL
		ORDER BY account_nicknames.created_at ASC
		LIMIT 1)
) COLLATE "pt-BR-x-icu" %s NULLS LAST, accounts.created_at ASC`

var accountOrder = map[string]string{
	"name_asc":     fmt.Sprintf(accountLabelOrder, "ASC"),
	"name_desc":    fmt.Sprintf(accountLabelOrder, "DESC"),
	"created_desc": "accounts.created_at DESC",
	"created_asc":  "accounts.created_at ASC",
}

// accountListQuery applies the optional search and the ordering. The join to holders is
// unconditional because the default ordering reads from it.
func accountListQuery(db *gorm.DB, params listQuery, programID, category string, complete *bool) *gorm.DB {
	query := db.Model(&data.Account{}).
		Joins("LEFT JOIN people AS holders ON holders.id = accounts.holder_id")

	if params.Q != "" {
		like := likePattern(params.Q)
		search := db.Where(unaccentedLike("accounts.login"), like).
			Or(unaccentedLike("holders.name"), like).
			Or("EXISTS (SELECT 1 FROM people AS administrators WHERE administrators.id = accounts.administrator_id AND "+
				unaccentedLike("administrators.name")+")", like).
			Or("EXISTS (SELECT 1 FROM loyalty_programs WHERE loyalty_programs.id = accounts.loyalty_program_id AND ("+
				unaccentedLike("loyalty_programs.name")+" OR "+unaccentedLike("loyalty_programs.code")+"))", like, like).
			Or(nicknameMatch("account_nicknames", "account_id", "accounts", true), like)
		query = query.Where(search)
	}

	if programID != "" {
		query = query.Where("accounts.loyalty_program_id = ?", programID)
	}
	if category != "" {
		query = query.Where("accounts.category = ?", category)
	}
	if complete != nil {
		query = query.Where("accounts.complete = ?", *complete)
	}

	return query.Order(orderClause(params.Order, accountOrder, accountOrder["name_asc"]))
}

// GetAccounts handles the GET /accounts route.
// @Summary      Listar contas
// @Description  Retorna todas as contas cadastradas no sistema.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        q                   query  string  false  "Busca por login, titular, administrador, programa ou apelido"
// @Param        order               query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        category            query  string  false  "Categoria da conta"  Enums(unidentified, standard, injunction)
// @Param        complete            query  bool    false  "Somente contas completas (true) ou incompletas (false)"
// @Param        include_archived    query  bool    false  "Inclui as contas arquivadas — usado pela tela de mesclagem, que precisa alcançá-las"
// @Success      200  {object}  schemas.AccountListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar contas"
// @Router       /accounts [get]
func (h *AccountHandler) GetAccounts(w http.ResponseWriter, r *http.Request) {
	var accounts []data.Account

	db := h.DB
	// Only the merge picker asks for this. Archiving was the only way to get rid of a
	// duplicate before merging existed, so the duplicates that most need absorbing are
	// exactly the archived ones - and their issuances are still sitting on them. Nothing
	// new is exposed: the issuances grid already receives archived accounts embedded in
	// old rows, which is why AccountResponse carries deleted_at at all.
	if archived := parseOptionalBool(r.URL.Query().Get("include_archived")); archived != nil && *archived {
		db = db.Unscoped()
	}

	picker := r.URL.Query().Get("view") == "picker"

	query := accountListQuery(
		db,
		parseListQuery(r),
		r.URL.Query().Get("loyalty_program_id"),
		r.URL.Query().Get("category"),
		parseOptionalBool(r.URL.Query().Get("complete")),
	).Preload("Nicknames")

	// The picker identifies a program by id and looks the rest up on the panel, so the
	// join that hydrates one program per account buys nothing here.
	if !picker {
		query = query.Preload("LoyaltyProgram")
	}

	if err := query.Find(&accounts).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch accounts")
		return
	}

	// view=picker answers with a projection rather than the whole account. The route is
	// unpaginated - it feeds the pickers on the issuances grid, the sales screens, the
	// importer and the merge panel - so every one of those screens used to carry every
	// column of every account in the system to render a list of names.
	//
	// The password is the one that mattered beyond bytes. Account.Password comes back
	// decrypted on purpose, because staff have to read it to sign into the airline's
	// program, and the account cards on the management screen are where that happens.
	// It was also going into the HTML of every screen that merely has a picker: a copy
	// of every account credential sitting in pages with no use for it.
	//
	// See schemas.AccountPickerResponse for what survives and why.
	var usage map[string]*acct.Usage
	if !picker {
		computed, err := h.cpfUsage(accounts)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to compute the CPF usage")
			return
		}
		usage = computed
	}

	if picker {
		resp := make([]schemas.AccountPickerResponse, 0, len(accounts))
		for _, acc := range accounts {
			resp = append(resp, toAccountPickerResponse(acc))
		}

		utils.SendJSON(w, r, http.StatusOK, "Accounts fetched successfully", resp)
		return
	}

	resp := make([]schemas.AccountResponse, 0, len(accounts))
	for _, acc := range accounts {
		resp = append(resp, toAccountResponseWithUsage(acc, usage[acc.ID]))
	}
	fillExpectedBalances(h.DB, resp)
	fillLastSynced(h.DB, resp)

	utils.SendJSON(w, r, http.StatusOK, "Accounts fetched successfully", resp)
}

// ListAccounts handles the GET /accounts/manage route.
// @Summary      Listar contas (paginado)
// @Description  Listagem paginada para a tela de gestão. A rota /accounts continua devolvendo tudo, porque é ela que alimenta os seletores da grid de emissões, das vendas, do relatório de fornecedores e da importação.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        q                   query  string  false  "Busca por login, titular, administrador, programa ou apelido"
// @Param        order               query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        category            query  string  false  "Categoria da conta"  Enums(unidentified, standard, injunction)
// @Param        complete            query  bool    false  "Somente contas completas (true) ou incompletas (false)"
// @Param        page                query  int     false  "Página (padrão 1)"
// @Param        limit               query  int     false  "Itens por página (padrão 20, máximo 100)"
// @Success      200  {object}  schemas.AccountPaginatedResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar contas"
// @Router       /accounts/manage [get]
func (h *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	params := parseListQuery(r)
	query := accountListQuery(
		h.DB,
		params,
		r.URL.Query().Get("loyalty_program_id"),
		r.URL.Query().Get("category"),
		parseOptionalBool(r.URL.Query().Get("complete")),
	)

	// The count runs on the same filtered query as the page, so the total the
	// pagination control shows can never disagree with what it is paginating.
	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count accounts")
		return
	}

	var accounts []data.Account
	if err := paginate(query.Session(&gorm.Session{}), params).
		Preload("Nicknames").Preload("LoyaltyProgram").
		Find(&accounts).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch accounts")
		return
	}

	// Only for the rows on screen. This is the one listing whose per-row extra is a
	// second query over the beneficiary ledger, so paging it is not just less JSON -
	// it is less counting.
	usage, err := h.cpfUsage(accounts)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to compute the CPF usage")
		return
	}

	items := make([]schemas.AccountResponse, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, toAccountResponseWithUsage(account, usage[account.ID]))
	}
	fillExpectedBalances(h.DB, items)
	fillLastSynced(h.DB, items)

	utils.SendJSON(w, r, http.StatusOK, "Accounts fetched successfully", schemas.AccountPaginatedData{
		Accounts: items,
		Total:    total,
		Page:     params.Page,
		Limit:    params.Limit,
	})
}

// cpfUsage computes the beneficiary-cap consumption of a whole listing at once. The
// accounts must already carry their preloaded LoyaltyProgram - that is where the limit,
// the window and the counting mode live.
//
// One query for the listing rather than one per row: the accounts screen renders every
// account the operator has, and a per-card query would be a query per card.
func (h *AccountHandler) cpfUsage(accounts []data.Account) (map[string]*acct.Usage, error) {
	programs := make(map[string]data.LoyaltyProgram, len(accounts))
	for _, account := range accounts {
		if account.LoyaltyProgram != nil {
			programs[account.LoyaltyProgramID] = *account.LoyaltyProgram
		}
	}
	return acct.UsageForMany(h.DB, accounts, programs)
}

// GetAccount handles the GET /accounts/{id} route
// @Summary      Buscar conta por ID
// @Description  Retorna os dados de uma conta específica identificada pelo UUID.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da conta"
// @Success      200  {object}  schemas.AccountSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Router       /accounts/{id} [get]
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var account data.Account
	if err := h.DB.Preload("Nicknames").Preload("LoyaltyProgram").First(&account, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Account not found")
		return
	}

	usage, err := h.cpfUsage([]data.Account{account})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to compute the CPF usage")
		return
	}

	fetched := []schemas.AccountResponse{toAccountResponseWithUsage(account, usage[account.ID])}
	fillExpectedBalances(h.DB, fetched)
	fillLastSynced(h.DB, fetched)

	utils.SendJSON(w, r, http.StatusOK, "Account fetched successfully", fetched[0])
}

// UpdateAccount handles the PUT /accounts/{id} route
// @Summary      Atualizar conta
// @Description  Atualiza os dados de uma conta existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Contas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                      true  "UUID da conta"
// @Param        payload body  schemas.UpdateAccountRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.AccountSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Programa de fidelidade, titular ou administrador informado não existe"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar conta"
// @Router       /accounts/{id} [put]
func (h *AccountHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var account data.Account
	if err := h.DB.Preload("Nicknames").First(&account, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Account not found")
		return
	}

	var req schemas.UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.LoyaltyProgramID != nil {
		account.LoyaltyProgramID = *req.LoyaltyProgramID
	}
	if req.Login != nil {
		account.Login = req.Login
	}
	if req.Password != nil {
		account.Password = req.Password
	}
	if req.HolderID != nil {
		account.HolderID = req.HolderID
	}
	if req.AdministratorID != nil {
		account.AdministratorID = req.AdministratorID
	}
	if req.WaivesRedemptionFee != nil {
		account.WaivesRedemptionFee = *req.WaivesRedemptionFee
	}
	if req.OwnsPointInventory != nil {
		account.OwnsPointInventory = *req.OwnsPointInventory
	}
	if req.TracksStatement != nil {
		account.TracksStatement = *req.TracksStatement
	}
	if req.DefaultProfitSharePercent != nil {
		account.DefaultProfitSharePercent = *req.DefaultProfitSharePercent
	}
	if req.Category != nil {
		account.Category = *req.Category
	}
	// The real balance is not written from here anymore - the Azul statement sync owns it.

	if req.Complete != nil {
		account.Complete = *req.Complete
	} else {
		account.Complete = account.ComputeComplete()
	}

	if err := h.DB.Save(&account).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Programa de fidelidade, titular ou administrador informado não existe")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update account: "+err.Error())
		return
	}

	// Reload: BeforeSave leaves Password encrypted in memory, and only a fresh
	// read triggers AfterFind to decrypt it back for the response.
	if err := h.DB.Preload("Nicknames").Preload("LoyaltyProgram").First(&account, "id = ?", account.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load updated account: "+err.Error())
		return
	}

	updated := []schemas.AccountResponse{toAccountResponse(account)}
	fillExpectedBalances(h.DB, updated)
	fillLastSynced(h.DB, updated)

	utils.SendJSON(w, r, http.StatusOK, "Account updated successfully", updated[0])
}

// DeleteAccount handles the DELETE /accounts/{id} route.
// @Summary      Deletar conta
// @Description  Arquiva uma conta: ela some das listagens e dos seletores, mas as emissões antigas continuam mostrando em que conta foram feitas. Os apelidos da conta são arquivados junto, liberando o nome para outra conta do mesmo programa.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da conta"
// @Success      200  {object}  schemas.ErrorResponse "Conta arquivada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Conta ainda referenciada por outro registro"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao arquivar conta"
// @Router       /accounts/{id} [delete]
func (h *AccountHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// One transaction for the two, because an account whose aliases outlived it is
	// worse than one that was never archived: the alias is how the bot routes an
	// EMISSAO, so it would keep sending new issuances to an account nobody can see.
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", id).Delete(&data.AccountNickname{}).Error; err != nil {
			return err
		}
		return tx.Delete(&data.Account{}, "id = ?", id).Error
	})

	if err != nil {
		// Still reachable, just no longer through the issuance: people is referenced by
		// holder_id/administrator_id with RESTRICT and other rows may point here too.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir conta ainda referenciada por outro registro")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete account: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Account deleted successfully", nil)
}
