package schemas

import "time"

// The Smiles statement DTOs. They deliberately mirror the Azul ones field for field wherever
// the two programmes say the same thing, so the panel reconciles both through one shape
// instead of two near-identical screens. Where they differ, they differ because the airlines
// do - noted at each field.

// SmilesAuthenticator is one second-factor channel the account holder registered, as the panel
// offers it. Target is already masked by internal/smiles: the panel needs to say which phone or
// inbox to check, never the number itself.
type SmilesAuthenticator struct {
	ID string `json:"id"`
	// Channel is "sms" or "email".
	Channel string `json:"channel" example:"sms"`
	Target  string `json:"target" example:"***77"`
}

// SmilesConnectResponse is what starting a connection returns. Exactly one of the two paths is
// taken: either the account logged in outright, or it needs a verification code and the panel
// must ask which channel to send it on.
type SmilesConnectResponse struct {
	// Connected is true when the account had no second factor and is already usable.
	Connected bool `json:"connected" example:"false"`
	// MFARequired is true when a code is needed. The panel then posts a channel to
	// /smiles-connect/challenge and the typed digits to /smiles-connect/code.
	MFARequired    bool                  `json:"mfa_required" example:"true"`
	Authenticators []SmilesAuthenticator `json:"authenticators"`
}

// SmilesConnectSingleResponse is the connect route's envelope.
type SmilesConnectSingleResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Escolha onde receber o código"`
	Data    SmilesConnectResponse `json:"data"`
}

// SmilesChallengeRequest asks Smiles to send the code on one of the listed channels.
type SmilesChallengeRequest struct {
	AuthenticatorID string `json:"authenticator_id" validate:"required"`
}

// SmilesChallengeResponse confirms the code was sent, and where to - masked.
type SmilesChallengeResponse struct {
	Channel string `json:"channel" example:"sms"`
	SentTo  string `json:"sent_to" example:"***77"`
}

// SmilesChallengeSingleResponse is the challenge route's envelope.
type SmilesChallengeSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Código enviado"`
	Data    SmilesChallengeResponse `json:"data"`
}

// SmilesBrowserChallengeRequest picks the channel the operator's own browser will use to ask
// Smiles for the code. That browser is the extension: the edge refuses every client that is not
// a real browser on that POST, and the operator's own Chrome is the one client it treats as a
// person. This call hands the material over; the browser answers on /challenge/complete.
type SmilesBrowserChallengeRequest struct {
	AuthenticatorID string `json:"authenticator_id" validate:"required"`
}

// SmilesBrowserChallengeResponse is the material the extension needs: the short-lived token the
// tenant issued and the public client id. Both are only useful for this one challenge; the
// account's password never travels with them.
type SmilesBrowserChallengeResponse struct {
	MFAToken        string `json:"mfa_token"`
	ClientID        string `json:"client_id"`
	AuthenticatorID string `json:"authenticator_id"`
	Channel         string `json:"channel" example:"sms"`
	SentTo          string `json:"sent_to" example:"***77"`
}

// SmilesBrowserChallengeSingleResponse is the browser challenge route's envelope.
type SmilesBrowserChallengeSingleResponse struct {
	Success bool                           `json:"success" example:"true"`
	Message string                         `json:"message" example:"Peça o código pelo navegador"`
	Data    SmilesBrowserChallengeResponse `json:"data"`
}

// SmilesBrowserChallengeCompleteRequest hands back the oob_code the operator's browser got from
// the tenant. It is what ties the digits the operator will type to the challenge that was made.
type SmilesBrowserChallengeCompleteRequest struct {
	AuthenticatorID string `json:"authenticator_id" validate:"required"`
	OOBCode         string `json:"oob_code" validate:"required"`
}

// SmilesCodeRequest carries the digits the operator typed. Bounded rather than fixed at six:
// the length is the tenant's to choose and is not worth a 422 of our own invention.
type SmilesCodeRequest struct {
	Code string `json:"code" validate:"required,min=4,max=12"`
}

// SmilesSyncStateResponse is the account's statement-sync lifecycle as the panel reads it.
//
// It carries the same JSON field names as AzulSyncStateResponse on purpose, including
// last_balance_points for what Smiles calls miles: the panel has one badge component and one
// vocabulary, and splitting the shape would split the screen too.
type SmilesSyncStateResponse struct {
	// Connected is whether a session is stored for the account. False means it has never been
	// connected, or was disconnected, so the panel offers "Conectar".
	Connected bool `json:"connected" example:"true"`
	// State is one of pending|ok|error|needs_mfa|needs_attention (see data.SmilesSync*). There
	// is no "inconsistent" here: Smiles publishes no end-of-day balance to check a window
	// against, so there is nothing that could disagree.
	State string `json:"state" example:"ok"`
	// LastSyncedAt and LastBalancePoints are the last successful read; null until the first one.
	LastSyncedAt      *time.Time `json:"last_synced_at"`
	LastBalancePoints *int       `json:"last_balance_points" example:"1234567"`
	// ConsecutiveFailures is how many rounds have failed in a row; it is what tips the state
	// into needs_attention once it crosses the threshold.
	ConsecutiveFailures int `json:"consecutive_failures" example:"0"`
	// Detail is a short human note for the panel (the last error, or what the operator has to
	// do about needs_mfa). Null when there is nothing to say.
	Detail    *string    `json:"detail"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// SmilesSyncStateSingleResponse is the status route's envelope.
type SmilesSyncStateSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Estado carregado"`
	Data    SmilesSyncStateResponse `json:"data"`
}

// SmilesStatementEntry is one synced statement line, shown verbatim on the statement page,
// with the issuance or batch it resolves to.
//
// One field of the Azul entry is absent rather than null: note, because there is no free-text
// comment to parse one out of. Two are new: sub_type, which is how an estorno is spelled here,
// and channel.
type SmilesStatementEntry struct {
	// Key is what a manual link is stored under: the airline's own identifier for the
	// movement when it has one, otherwise a content signature. The page sends it back to link
	// or unlink this line.
	Key        string `json:"key" example:"IOTFQF"`
	OccurredOn string `json:"occurred_on" example:"2026-08-28"`
	// Kind is the movement in the panel's terms: redemption|refund|accrual|purchase|other.
	Kind     string  `json:"kind" example:"redemption"`
	TypeCode *string `json:"type_code" example:"Redemption"`
	// SubType is the airline's own sub-classification. "Cancellation" on a credit is the only
	// way this API spells an estorno.
	SubType         *string `json:"sub_type" example:"Product"`
	TypeDescription *string `json:"type_description" example:"VOO"`
	// Points carries our sign (negative when miles left the account); the payload is unsigned.
	Points int `json:"points" example:"-27400"`
	// Locator, Origin and Destination describe a flight movement; empty on a plain credit.
	// The airport codes are IATA straight from the airline - there is no city table here.
	Locator     string `json:"locator" example:"IOTFQF"`
	Origin      string `json:"origin" example:"GYN"`
	Destination string `json:"destination" example:"GRU"`
	PartnerName string `json:"partner_name" example:"LIVELO S.A."`
	Description string `json:"description" example:"Pontos Livelo"`
	// Channel is where the movement was made ("Web", "APP", "MSR"). It is what tells an
	// operator's own booking from one the airline generated.
	Channel string `json:"channel" example:"Web"`
	// DayBalance is what the account held at the end of the day - DERIVED by the sync, because
	// Smiles reports no such figure. It reads the same as Azul's field and means something
	// weaker: it is built from these very movements, so it can never contradict them. Shown,
	// never checked.
	DayBalance *int `json:"day_balance" example:"1234567"`
	// LinkedIssuance is the resolved issuance (for redemptions/refunds), or null when nothing
	// matched. LinkedBatch is the resolved batch (for purchases/accruals), or null.
	LinkedIssuance *AzulStatementLinkedIssuance `json:"linked_issuance"`
	LinkedBatch    *AzulStatementLinkedBatch    `json:"linked_batch"`
}

// SmilesStatementEntriesResponse is the per-account statement page: the badge, the window's
// span and the lines.
type SmilesStatementEntriesResponse struct {
	SyncState SmilesSyncStateResponse `json:"sync_state"`
	From      string                  `json:"from" example:"2026-08-18"`
	To        string                  `json:"to" example:"2026-09-17"`
	Lines     []SmilesStatementEntry  `json:"lines"`
}

// SmilesStatementEntriesSingleResponse is the statement-entries route's envelope.
type SmilesStatementEntriesSingleResponse struct {
	Success bool                           `json:"success" example:"true"`
	Message string                         `json:"message" example:"Extrato carregado"`
	Data    SmilesStatementEntriesResponse `json:"data"`
}

// SmilesStatementResponse is the reconciliation of the synced window against the panel, plus
// the badge - the shape the inventory screen reads.
type SmilesStatementResponse struct {
	SyncState      SmilesSyncStateResponse         `json:"sync_state"`
	Reconciliation StatementReconciliationResponse `json:"reconciliation"`
}

// SmilesStatementSingleResponse is the reconciliation route's envelope.
type SmilesStatementSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Extrato conciliado"`
	Data    SmilesStatementResponse `json:"data"`
}

// StatementSmilesCommitRequest records the chosen credits of the synced window as batches. It
// is the twin of StatementAzulCommitRequest: no file, just the selection.
type StatementSmilesCommitRequest struct {
	Batches []StatementBatchSelection `json:"batches" validate:"required,min=1,dive"`
}
