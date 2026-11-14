package promotion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrAzulLoginFailed is a login that reached the service but did not produce a customerKey -
// wrong credentials, or the site turning the login away. Distinct
// from a transport failure so the caller can tell "try again / fix the password" from "the
// promo service is down".
var ErrAzulLoginFailed = errors.New("promotion: azul login did not produce a customerKey")

// The reasons the login service gives for a login that did not produce a customerKey. The first
// four are about the account and count toward its failure streak; the rest are the service, the
// Azul edge or the environment, and say nothing about the credentials.
const (
	AzulLoginReasonCredentials = "credentials" // Azul refused the login and password (401)
	AzulLoginReasonLocked      = "locked"      // Azul locked the account (423)
	AzulLoginReasonRejected    = "rejected"    // any other refusal of this account's login
	AzulLoginReasonNoKey       = "no_key"      // the login passed but returned no customerKey
	AzulLoginReasonBlocked     = "blocked"     // the edge refused the shared device or the rate
	AzulLoginReasonDisabled    = "disabled"    // the login is switched off in this environment
	AzulLoginReasonBusy        = "busy"        // the service's login queue is full
	AzulLoginReasonUnavailable = "unavailable" // the service failed or could not be reached
)

// AzulLoginError is a login that did not produce a customerKey, with the service's reason. It
// matches ErrAzulLoginFailed only for the reasons about the account, so errors.Is keeps telling
// "fix the password" from "try again later" without the caller knowing the reasons.
type AzulLoginError struct {
	Reason string
	// Message is the service's own credential-free message, for the logs.
	Message string
	// Err is the underlying transport error, when there was one.
	Err error
}

func (e *AzulLoginError) Error() string {
	text := "promotion: azul login " + e.Reason
	if e.Message != "" {
		text += ": " + e.Message
	}
	if e.Err != nil {
		text += ": " + e.Err.Error()
	}
	return text
}

func (e *AzulLoginError) Unwrap() error { return e.Err }

// Is makes errors.Is(err, ErrAzulLoginFailed) true only for an outcome about the account.
func (e *AzulLoginError) Is(target error) bool {
	return target == ErrAzulLoginFailed && e.IsAccountOutcome()
}

// IsAccountOutcome reports whether the failure is about this account's credentials or status.
func (e *AzulLoginError) IsAccountOutcome() bool {
	switch e.Reason {
	case AzulLoginReasonCredentials, AzulLoginReasonLocked, AzulLoginReasonRejected, AzulLoginReasonNoKey:
		return true
	}
	return false
}

// AzulLoginRequest carries the account's stored credentials to the login service, which
// returns the account's customerKey.
type AzulLoginRequest struct {
	DocumentNumber string `json:"document_number"`
	Password       string `json:"password"`
}

type azulLoginResponse struct {
	CustomerKey string `json:"customer_key"`
	Error       string `json:"error"`
	Reason      string `json:"reason"`
}

// AzulLogin asks the promo service to log the account in and return its customerKey. It is the
// one-time connection step; the ongoing statement sync only reads and needs no browser.
//
// The credentials are sent over the internal Compose network only and are never logged here.
func (c *Client) AzulLogin(ctx context.Context, req AzulLoginRequest) (string, error) {
	//nolint:gosec // G117: the password must reach the login service to be typed into the form;
	// this call stays on the internal Compose network and the value is never logged.
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("promotion: encoding azul login request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.azulLoginURL+"/login", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("promotion: building azul login request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", &AzulLoginError{Reason: AzulLoginReasonUnavailable, Message: "service unreachable", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	var decoded azulLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", &AzulLoginError{
			Reason:  AzulLoginReasonUnavailable,
			Message: fmt.Sprintf("HTTP %d with an unreadable body", resp.StatusCode),
			Err:     err,
		}
	}
	// The service answers 502 for a login outcome about the account (bad
	// password, a 200 without a key), 503 when the Azul edge refused the shared
	// device or the rate, and 400/409/500 for its own failures. Only the 502 is a
	// credential outcome the panel should count; an edge block, a busy queue or a
	// dead service must not look like a wrong password.
	//
	// The status decides whether the failure can count against the account; the body's reason
	// only refines the message within that class. A reason that contradicts the status (an
	// account reason on a 503, say) is ignored, so a confused service can never mark accounts.
	if resp.StatusCode != http.StatusOK {
		return "", &AzulLoginError{
			Reason:  reasonForStatus(resp.StatusCode, decoded.Reason),
			Message: decoded.Error,
		}
	}
	if decoded.CustomerKey == "" {
		// 200 but no key is still a failure - the login never surfaced a customerKey.
		return "", &AzulLoginError{Reason: AzulLoginReasonNoKey}
	}
	return decoded.CustomerKey, nil
}

func reasonForStatus(status int, reason string) string {
	account := &AzulLoginError{Reason: reason}
	switch status {
	case http.StatusBadGateway:
		if account.IsAccountOutcome() {
			return reason
		}
		return AzulLoginReasonRejected
	case http.StatusServiceUnavailable:
		if reason == AzulLoginReasonDisabled {
			return reason
		}
		return AzulLoginReasonBlocked
	case http.StatusConflict:
		return AzulLoginReasonBusy
	default:
		return AzulLoginReasonUnavailable
	}
}
