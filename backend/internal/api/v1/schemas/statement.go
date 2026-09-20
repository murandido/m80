package schemas

import "time"

// StatementBookingLine compares one booking between the statement and the panel.
type StatementBookingLine struct {
	Locator string `json:"locator" example:"MJF6HC"`
	// Verdict is matched, missing_in_panel, missing_in_statement or amount_differs.
	Verdict string `json:"verdict" example:"amount_differs"`
	// StatementPoints is what left the account according to the airline, positive, with
	// the legs of one booking already summed.
	StatementPoints int    `json:"statement_points" example:"360000"`
	PanelPoints     int    `json:"panel_points" example:"350000"`
	IssuanceID      string `json:"issuance_id"`
	OccurredOn      string `json:"occurred_on" example:"2026-08-05"`
}

// StatementRefundLine compares one booking's refund.
type StatementRefundLine struct {
	Locator         string `json:"locator" example:"YW51QG"`
	Verdict         string `json:"verdict" example:"amount_differs"`
	StatementPoints int    `json:"statement_points" example:"40000"`
	PanelPoints     int    `json:"panel_points" example:"0"`
	IssuanceID      string `json:"issuance_id"`
}

// StatementCreditLine is one credit on the statement, paired with a batch or not.
//
// A credit with verdict missing_in_panel is stock that arrived and was never priced: the
// costing is spending it at whatever the neighbouring batch cost. It is the line this
// screen exists to surface, and the only one the commit route acts on.
type StatementCreditLine struct {
	// Index identifies the credit within this statement, and is what the commit route
	// takes. The client never sends the credit's contents back - the file is read and
	// reconciled again from scratch, so the preview is advice, not contract.
	Index       int    `json:"index" example:"0"`
	OccurredOn  string `json:"occurred_on" example:"2026-08-27"`
	Points      int    `json:"points" example:"158175"`
	Description string `json:"description" example:"Resgate pontos Inter"`
	Verdict     string `json:"verdict" example:"missing_in_panel"`
	BatchID     string `json:"batch_id"`
}

// StatementReconciliationResponse is the whole comparison.
type StatementReconciliationResponse struct {
	// AccountNumber is the programme's own number for the account, when the file states
	// it. Advisory: it lets the screen warn that the file may be from another account.
	AccountNumber string `json:"account_number" example:"03303567123"`
	// From and To are the statement's own span, not a window anybody chose.
	From string `json:"from" example:"2026-07-22"`
	To   string `json:"to" example:"2026-09-07"`

	Bookings []StatementBookingLine `json:"bookings"`
	Refunds  []StatementRefundLine  `json:"refunds"`
	Credits  []StatementCreditLine  `json:"credits"`

	// StatementNetPoints and PanelNetPoints are the change in balance each side claims.
	// Neither export states a balance, so agreeing on the change is the only check there
	// is - and disagreeing means something above was missed.
	StatementNetPoints int `json:"statement_net_points" example:"-158800"`
	PanelNetPoints     int `json:"panel_net_points" example:"-158800"`
}

// StatementReconciliationSingleResponse is the preview route's envelope.
type StatementReconciliationSingleResponse struct {
	Success bool                            `json:"success" example:"true"`
	Message string                          `json:"message" example:"Extrato conciliado"`
	Data    StatementReconciliationResponse `json:"data"`
}

// StatementBatchSelection is one credit the operator chose to record, with the two things
// the statement cannot know: what it cost and where it came from.
type StatementBatchSelection struct {
	Index    int    `json:"index" example:"0"`
	OriginID string `json:"origin_id" validate:"required,uuid4"`
	// CostCents is what this credit cost. Zero is a real answer - a club subscription and
	// flight accrual both arrive for nothing.
	CostCents int `json:"cost_cents" validate:"min=0" example:"224775"`
	// FundingAgencyID is the agency that paid for it.
	FundingAgencyID string `json:"funding_agency_id" validate:"required,uuid4"`
	// Note travels onto the batch, for the operator's own labelling.
	Note string `json:"note"`
}

// StatementAzulCommitRequest is the body of the Azul commit route. Unlike the Smiles route,
// the Azul statement is not uploaded - it is read from the synced window - so the request
// carries only the selected credits, as a JSON body rather than a multipart form.
type StatementAzulCommitRequest struct {
	Batches []StatementBatchSelection `json:"batches" validate:"required,min=1,dive"`
}

// AzulStatementLinkedIssuance is the real issuance a statement line resolves to, summarised for
// the statement page. Manual says whether the mapping was pinned by hand (true) or matched
// automatically by locator (false). Null on the entry when the line has no locator or nothing
// matched.
type AzulStatementLinkedIssuance struct {
	IssuanceID    string `json:"issuance_id"`
	PassengerName string `json:"passenger_name" example:"MARIA SILVA"`
	Origin        string `json:"origin" example:"GRU"`
	Destination   string `json:"destination" example:"REC"`
	IssuedOn      string `json:"issued_on" example:"2026-08-27"`
	Points        int    `json:"points" example:"25000"`
	Manual        bool   `json:"manual" example:"false"`
}

// AzulStatementLinkedBatch is the point batch (lote) a purchase or accrual line resolves to,
// summarised for the statement page. Manual says whether the mapping was pinned by hand (true) or
// matched automatically by the batch's recorded credits (false). Null on the entry when nothing
// matched.
type AzulStatementLinkedBatch struct {
	BatchID    string `json:"batch_id"`
	Origin     string `json:"origin" example:"Livelo"`
	AcquiredOn string `json:"acquired_on" example:"2026-08-27"`
	Points     int    `json:"points" example:"158175"`
	CostCents  int    `json:"cost_cents" example:"224775"`
	Manual     bool   `json:"manual" example:"false"`
}

// AzulStatementEntry is one synced statement line, shown verbatim on the statement page, with
// the issuance or batch it resolves to. Unlike the reconciliation lines, these are the raw
// movements (every kind), in day order.
type AzulStatementEntry struct {
	// Key is what a manual link is stored under: the locator when the line has one, otherwise a
	// content signature. The page sends it back to link or unlink this line.
	Key        string `json:"key" example:"MJF6HC"`
	OccurredOn string `json:"occurred_on" example:"2026-08-27"`
	// Kind is the movement in the panel's terms: redemption|refund|accrual|purchase|expiration|other.
	Kind            string  `json:"kind" example:"redemption"`
	TypeCode        *string `json:"type_code" example:"AN"`
	TypeDescription *string `json:"type_description"`
	// Points carries the airline's own sign (negative when points left the account).
	Points      int    `json:"points" example:"-25000"`
	Locator     string `json:"locator" example:"MJF6HC"`
	Origin      string `json:"origin" example:"GRU"`
	Destination string `json:"destination" example:"REC"`
	PartnerName string `json:"partner_name"`
	Description string `json:"description" example:"Resgate de passagem"`
	DayBalance  *int   `json:"day_balance" example:"1202717"`
	// LinkedIssuance is the resolved issuance (for redemptions/refunds/promotions), or null when
	// nothing matched. LinkedBatch is the resolved batch (for purchases/accruals), or null. A
	// line carries at most one of them.
	LinkedIssuance *AzulStatementLinkedIssuance `json:"linked_issuance"`
	LinkedBatch    *AzulStatementLinkedBatch    `json:"linked_batch"`
}

// AzulStatementEntriesResponse is the statement page's payload: the sync state (for the header),
// the synced window's span, and the lines in day order.
type AzulStatementEntriesResponse struct {
	SyncState AzulSyncStateResponse `json:"sync_state"`
	From      string                `json:"from" example:"2026-08-18"`
	To        string                `json:"to" example:"2026-09-17"`
	Lines     []AzulStatementEntry  `json:"lines"`
}

// AzulStatementEntriesSingleResponse is the statement-entries route's envelope.
type AzulStatementEntriesSingleResponse struct {
	Success bool                         `json:"success" example:"true"`
	Message string                       `json:"message" example:"Extrato carregado"`
	Data    AzulStatementEntriesResponse `json:"data"`
}

// AzulBookingLinkRequest pins a statement locator to a real issuance. Both are required.
type AzulBookingLinkRequest struct {
	Locator    string `json:"locator" validate:"required" example:"MJF6HC"`
	IssuanceID string `json:"issuance_id" validate:"required,uuid4"`
}

// AzulBookingLinkDeleteRequest removes a manual pin, handing the locator back to the automatic
// match.
type AzulBookingLinkDeleteRequest struct {
	Locator string `json:"locator" validate:"required" example:"MJF6HC"`
}

// AzulBookingLinkResponse is what the link routes return: the locator and how it now resolves,
// so the page can update the line in place. LinkedIssuance is null when nothing resolves (a
// delete that leaves no automatic match).
type AzulBookingLinkResponse struct {
	Locator        string                       `json:"locator" example:"MJF6HC"`
	LinkedIssuance *AzulStatementLinkedIssuance `json:"linked_issuance"`
}

// AzulBookingLinkSingleResponse is the link routes' envelope.
type AzulBookingLinkSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Vínculo salvo"`
	Data    AzulBookingLinkResponse `json:"data"`
}

// AzulBatchLinkRequest pins a statement line (by its key) to a point batch. occurred_on and
// points identify the line so the response can recompute the automatic match; the day is
// YYYY-MM-DD.
type AzulBatchLinkRequest struct {
	Key        string `json:"key" validate:"required" example:"sig:2026-08-28:ER:632700"`
	BatchID    string `json:"batch_id" validate:"required,uuid4"`
	OccurredOn string `json:"occurred_on" validate:"required,datetime=2006-01-02" example:"2026-08-28"`
	Points     int    `json:"points" example:"632700"`
}

// AzulBatchLinkDeleteRequest removes a manual batch pin, handing the line back to the automatic
// match. occurred_on and points let the response report the automatic match that reappears.
type AzulBatchLinkDeleteRequest struct {
	Key        string `json:"key" validate:"required"`
	OccurredOn string `json:"occurred_on" validate:"required,datetime=2006-01-02" example:"2026-08-28"`
	Points     int    `json:"points" example:"632700"`
}

// AzulBatchLinkResponse is what the batch-link routes return: the line key and how it now
// resolves, so the page can update the line in place.
type AzulBatchLinkResponse struct {
	Key         string                    `json:"key" example:"sig:2026-08-28:ER:632700"`
	LinkedBatch *AzulStatementLinkedBatch `json:"linked_batch"`
}

// AzulBatchLinkSingleResponse is the batch-link routes' envelope.
type AzulBatchLinkSingleResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Vínculo salvo"`
	Data    AzulBatchLinkResponse `json:"data"`
}

// AzulCityCodeRequest assigns an IATA code to a statement place name. iata is upper-cased and must
// be three letters.
type AzulCityCodeRequest struct {
	City string `json:"city" validate:"required" example:"Cidade Nova"`
	IATA string `json:"iata" validate:"required,len=3,alpha" example:"XYZ"`
}

// AzulCityCodeResponse echoes the assignment, so the page can apply it to every line with that
// city without a full refetch.
type AzulCityCodeResponse struct {
	City string `json:"city" example:"Cidade Nova"`
	IATA string `json:"iata" example:"XYZ"`
}

// AzulCityCodeSingleResponse is the assign route's envelope.
type AzulCityCodeSingleResponse struct {
	Success bool                 `json:"success" example:"true"`
	Message string               `json:"message" example:"Código atribuído"`
	Data    AzulCityCodeResponse `json:"data"`
}

// AzulConnectResponse is what the connect route reports: the account's sync state after the
// connection (pending on success - the first real sync happens on the next ticker pass).
// AzulConnectResponse acknowledges that the background login was started. The connect is
// asynchronous, so there is no customerKey or final state to return yet - the panel watches the
// sync-state badge for the outcome.
type AzulConnectResponse struct {
	Connecting bool `json:"connecting" example:"true"`
}

// AzulSyncStateResponse is the account's statement-sync lifecycle as the panel reads it: whether
// it has ever been connected, the badge (state + detail), when the last round ran and the last
// balance it saw. Mirrors data.AzulSyncState; the Portuguese labels live on the panel.
type AzulSyncStateResponse struct {
	// Connected is whether a customerKey has been discovered for the account. False means it has
	// never been connected, so the panel offers "Conectar" rather than a running state.
	Connected bool `json:"connected" example:"true"`
	// Connecting is whether a background connect login is in flight right now. The connect is
	// asynchronous, so the panel shows "conectando" and polls this until it clears - then Connected
	// is true (success) or the state carries the failure.
	Connecting bool `json:"connecting" example:"false"`
	// ConnectingSince is when the in-flight login started, so the panel can show the elapsed time
	// (the login legitimately takes minutes across retries). Null whenever Connecting is false.
	ConnectingSince *time.Time `json:"connecting_since"`
	// State is one of pending|ok|error|inconsistent|needs_attention (see data.AzulSync*).
	State string `json:"state" example:"ok"`
	// LastSyncedAt and LastBalancePoints are the last successful read; null until the first one.
	LastSyncedAt      *time.Time `json:"last_synced_at"`
	LastBalancePoints *int       `json:"last_balance_points"`
	// ConsecutiveFailures is how many rounds (login or read) have failed in a row; it is what
	// tips the state into needs_attention once it crosses the threshold.
	ConsecutiveFailures int `json:"consecutive_failures" example:"0"`
	// Detail is a short human note for the panel (the last error, or a balance mismatch). Null
	// on an account that has nothing to report yet.
	Detail *string `json:"detail"`
	// UpdatedAt is when the state row last changed; null on an account with no row yet.
	UpdatedAt *time.Time `json:"updated_at"`
}

// AzulStatementResponse is what the Azul reconciliation route returns: the sync state (for the
// badge) alongside the reconciliation of the synced window. Unlike Smiles there is no upload, so
// both come from the database in a single read.
type AzulStatementResponse struct {
	SyncState      AzulSyncStateResponse           `json:"sync_state"`
	Reconciliation StatementReconciliationResponse `json:"reconciliation"`
}

// AzulStatementSingleResponse is the Azul reconciliation route's envelope.
type AzulStatementSingleResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Extrato conciliado"`
	Data    AzulStatementResponse `json:"data"`
}

// AzulConnectSingleResponse is the connect route's envelope.
type AzulConnectSingleResponse struct {
	Success bool                `json:"success" example:"true"`
	Message string              `json:"message" example:"Conta conectada"`
	Data    AzulConnectResponse `json:"data"`
}

// AzulSyncStateSingleResponse is the connect-status route's envelope: just the sync-state badge,
// cheap to poll. The account card reads it to know whether a connect is in flight without
// pulling the whole statement.
type AzulSyncStateSingleResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Estado carregado"`
	Data    AzulSyncStateResponse `json:"data"`
}

// StatementCommitResponse says what the commit wrote.
type StatementCommitResponse struct {
	// Created counts batches, not credits: credits of the same origin and payer are summed
	// into one batch, so four instalments of the same purchase come back as one.
	Created int `json:"created" example:"1"`
	// Credits counts how many statement lines went into them, which is what makes a commit
	// of twenty accruals into one batch readable instead of alarming.
	Credits int `json:"credits" example:"4"`
	// Skipped counts selections the reconciliation no longer reports as missing - a batch
	// entered between the preview and the commit, or the same file submitted twice.
	Skipped int `json:"skipped" example:"0"`
}

// StatementCommitSingleResponse is the commit route's envelope.
type StatementCommitSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Lotes lançados a partir do extrato"`
	Data    StatementCommitResponse `json:"data"`
}
