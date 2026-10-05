package schemas

import (
	"time"

	"github.com/murandido/m-80-backend/internal/account"
)

// CreateAccountRequest defines the expected payload to create a new account
type CreateAccountRequest struct {
	LoyaltyProgramID string  `json:"loyalty_program_id" validate:"required,uuid"`
	Login            *string `json:"login" validate:"omitempty"`
	Password         *string `json:"password" validate:"omitempty"`
	// HolderID is required: the panel shows an account as part of its holder's row, so one
	// without a holder has nowhere to belong. Older rows may still lack it; new ones cannot.
	HolderID        *string `json:"holder_id" validate:"required,uuid"`
	AdministratorID *string `json:"administrator_id" validate:"omitempty,uuid"`
	// Category decides whether the program's beneficiary cap applies. Omitted means
	// "unidentified", the honest default for an account nobody has classified.
	Category *string `json:"category" validate:"omitempty,oneof=unidentified standard injunction"`
	// WaivesRedemptionFee marks an account that pays no redemption fee - Azul's Diamante
	// Unique. Omitted means false, which is what every account paid before this existed.
	WaivesRedemptionFee *bool `json:"waives_redemption_fee" validate:"omitempty"`
	// OwnsPointInventory puts the account on the batch costing: its points were bought by
	// us, so their cost comes from the batches rather than from a rate typed on
	// /sales/suppliers. Omitted means false, which is every account that existed before
	// this - and what keeps the costing dormant until an account is deliberately marked.
	OwnsPointInventory *bool `json:"owns_point_inventory" validate:"omitempty"`
	// TracksStatement turns on reading this account's statement from the airline: connecting
	// it, the periodic sync and the statement screens. A different question from the flag
	// above - an account administered for someone else has a statement worth watching and no
	// inventory to cost. Omitted means false.
	TracksStatement *bool `json:"tracks_statement" validate:"omitempty"`
	// DefaultProfitSharePercent seeds the split on new batches of this account. Each batch
	// freezes its own copy, so changing this never revises a month already settled.
	DefaultProfitSharePercent *int `json:"default_profit_share_percent" validate:"omitempty,min=0,max=100"`
	// Complete overrides the automatically computed value when provided
	Complete *bool `json:"complete" validate:"omitempty"`
}

// UpdateAccountRequest defines the payload for updating an existing account.
// All fields are optional pointers: nil means "don't change this field".
type UpdateAccountRequest struct {
	LoyaltyProgramID *string `json:"loyalty_program_id" validate:"omitempty,uuid"`
	Login            *string `json:"login" validate:"omitempty"`
	Password         *string `json:"password" validate:"omitempty"`
	HolderID         *string `json:"holder_id" validate:"omitempty,uuid"`
	AdministratorID  *string `json:"administrator_id" validate:"omitempty,uuid"`
	Category         *string `json:"category" validate:"omitempty,oneof=unidentified standard injunction"`
	// WaivesRedemptionFee marks an account that pays no redemption fee - Azul's Diamante
	// Unique. Changing it does not rewrite what past issuances already recorded: the saving
	// is stored on the issuance and only moves when that issuance is written again. See
	// data.Issuance.RedemptionFeeSavedCents.
	WaivesRedemptionFee *bool `json:"waives_redemption_fee" validate:"omitempty"`
	// OwnsPointInventory puts the account on the batch costing: its points were bought by
	// us, so their cost comes from the batches rather than from a rate typed on
	// /sales/suppliers. Omitted means false, which is every account that existed before
	// this - and what keeps the costing dormant until an account is deliberately marked.
	OwnsPointInventory *bool `json:"owns_point_inventory" validate:"omitempty"`
	// TracksStatement turns on reading this account's statement from the airline: connecting
	// it, the periodic sync and the statement screens. A different question from the flag
	// above - an account administered for someone else has a statement worth watching and no
	// inventory to cost. Omitted means false.
	TracksStatement *bool `json:"tracks_statement" validate:"omitempty"`
	// DefaultProfitSharePercent seeds the split on new batches of this account. Each batch
	// freezes its own copy, so changing this never revises a month already settled.
	DefaultProfitSharePercent *int `json:"default_profit_share_percent" validate:"omitempty,min=0,max=100"`
	// The real balance is no longer editable here: it is owned by the Azul statement sync,
	// which reads it off the airline and writes accounts.real_balance_points itself (see
	// internal/azulsync). It stays on AccountResponse for reading. A program without a sync
	// yet (Smiles) simply has no real balance until its own sync ships.
	//
	// Complete overrides the automatically recomputed value when provided.
	Complete *bool `json:"complete" validate:"omitempty"`
}

// AccountResponse defines how an account should be returned in the API.
// Password is returned decrypted (unlike User's password, which is write-only)
// since staff need to see it to log into the loyalty program on the customer's behalf.
type AccountResponse struct {
	ID               string                    `json:"id"`
	LoyaltyProgramID string                    `json:"loyalty_program_id"`
	LoyaltyProgram   LoyaltyProgramResponse    `json:"loyalty_program"`
	Login            *string                   `json:"login"`
	Password         *string                   `json:"password"`
	HolderID         *string                   `json:"holder_id"`
	AdministratorID  *string                   `json:"administrator_id"`
	Nicknames        []AccountNicknameResponse `json:"nicknames"`
	// Category is "unidentified", "standard" or "injunction" - the Portuguese labels
	// live on the screen. See data.AccountCategory*.
	Category string `json:"category"`
	// WaivesRedemptionFee is true on an account exempt from the airline's redemption fee -
	// Azul's Diamante Unique. It is what turns a computed fee into "saved" instead of
	// "owed" on the issuances made through this account.
	WaivesRedemptionFee bool `json:"waives_redemption_fee"`
	// OwnsPointInventory is true on an account whose points cost is derived from its own
	// batches. The suppliers screen reads it to know the rate is not editable there.
	OwnsPointInventory bool `json:"owns_point_inventory"`
	// TracksStatement is true on an account whose statement is read from the airline. The
	// account card reads it to know whether to offer "Conectar".
	TracksStatement bool `json:"tracks_statement"`
	// DefaultProfitSharePercent is the split a new batch of this account starts with.
	DefaultProfitSharePercent int `json:"default_profit_share_percent"`
	// RealBalancePoints is the last balance read off the airline's page, and
	// RealBalanceCheckedOn the day it was read. Null until somebody looks.
	RealBalancePoints    *int    `json:"real_balance_points"`
	RealBalanceCheckedOn *string `json:"real_balance_checked_on" example:"2026-09-09"`
	// ExpectedBalancePoints is what the costing says the account still has: the batches,
	// less what was issued. Null on an account that does not own its point inventory -
	// there is no credit side to it in the panel, so a figure would be derived from half
	// a history. See inventory.ExpectedBalances.
	ExpectedBalancePoints *int `json:"expected_balance_points"`
	// AzulConnected is whether a customerKey has been discovered for this account - i.e.
	// whether the Azul statement sync can read it. The key itself is never returned (it is
	// json:"-" on the model), so this bit is how the panel knows to offer "Conectar" versus
	// showing the synced balance. Always false on accounts of other programs.
	AzulConnected bool `json:"azul_connected"`
	// AzulLastSyncedAt is when the statement sync last succeeded for this account, so the
	// card can show "atualizado há…". Null until the first successful sync, or on accounts
	// with no sync state row.
	AzulLastSyncedAt *time.Time `json:"azul_last_synced_at"`
	// SmilesConnected is whether a session is stored for this account - i.e. whether the
	// Smiles statement sync can read it. The session itself is never returned (it is
	// json:"-" on the model), so this bit is how the panel knows to offer "Conectar" versus
	// showing the synced balance. Always false on accounts of other programs.
	SmilesConnected bool `json:"smiles_connected"`
	// SmilesLastSyncedAt is the Smiles twin of AzulLastSyncedAt.
	SmilesLastSyncedAt *time.Time `json:"smiles_last_synced_at"`
	// CPFUsage is how much of the program's beneficiary cap this account has consumed,
	// and is null whenever there is no cap to report: an injunction suspends it, an
	// unidentified account gives no basis to claim one, and a program nobody has
	// researched has no limit. Null means "render no counter", not "zero".
	CPFUsage  *account.Usage `json:"cpf_usage"`
	Complete  bool           `json:"complete"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	// Only ever non-null on an account reached through a past issuance - the account
	// list itself never returns archived ones. It is what lets the grid label an old
	// row with an archived account while keeping it out of the picker.
	DeletedAt *time.Time `json:"deleted_at"`
}

// AccountNicknameOption is an alias as a picker uses it: something to type, and an id to
// send back with the choice. The full AccountNicknameResponse also repeats the account
// and program ids the caller already has, plus a timestamp nobody reads here.
type AccountNicknameOption struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

// AccountPickerResponse is everything a combobox needs to identify an account, and
// nothing else.
//
// Accounts have no name of their own, so a picker builds a label from the holder, the
// login and the aliases, colours it by program, and marks the archived ones. That is
// seven fields. The full AccountResponse carries sixteen, including a copy of the
// loyalty program embedded once per account - and on this unpaginated feed of eight
// hundred, that copy alone was half the bytes.
//
// The program arrives as an id. Every screen with an account picker already fetches the
// program list, which is 3 KB for all of them, so embedding one per account was paying
// hundreds of times for something already on the page.
type AccountPickerResponse struct {
	ID               string                  `json:"id"`
	LoyaltyProgramID string                  `json:"loyalty_program_id"`
	Login            *string                 `json:"login"`
	HolderID         *string                 `json:"holder_id"`
	Nicknames        []AccountNicknameOption `json:"nicknames"`
	// Both are omitted when they carry no information, which on a feed this size is most
	// rows: an account that waives no fee and was never archived says nothing by saying
	// false and null. The panel defaults them back, as it already did for absent fields.
	WaivesRedemptionFee bool `json:"waives_redemption_fee,omitempty"`
	// OwnsPointInventory is here so the inventory screen can offer only the accounts the
	// costing applies to - an ordinary account has no batches and never will, and listing
	// it would produce an empty screen with no way to tell why. Omitted when false for the
	// same reason as the flag above: on an unpaginated feed that is nearly every row.
	OwnsPointInventory bool `json:"owns_point_inventory,omitempty"`
	// TracksStatement is omitted when false for the same reason.
	TracksStatement bool       `json:"tracks_statement,omitempty"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

// AccountRemovalResponse says whether DELETE /accounts/{id} deleted the account for real or,
// because it carries history, archived it.
type AccountRemovalResponse struct {
	Deleted bool `json:"deleted"`
}

// AccountRemovalSingleResponse is the DELETE /accounts/{id} route response.
type AccountRemovalSingleResponse struct {
	Success bool                   `json:"success" example:"true"`
	Message string                 `json:"message" example:"Conta apagada"`
	Data    AccountRemovalResponse `json:"data"`
}
