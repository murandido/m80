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
// wrong credentials, or the behavioural/challenge gate turning headless Chromium away. Distinct
// from a transport failure so the caller can tell "try again / fix the password" from "the
// promo service is down".
var ErrAzulLoginFailed = errors.New("promotion: azul login did not produce a customerKey")

// AzulLoginRequest carries the account's stored credentials to the promo service, which logs in
// through the residential proxy and reads the customerKey off the SPA's own requests.
type AzulLoginRequest struct {
	DocumentNumber string `json:"document_number"`
	Password       string `json:"password"`
}

type azulLoginResponse struct {
	CustomerKey string `json:"customer_key"`
	Error       string `json:"error"`
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
		return "", fmt.Errorf("promotion: promo service unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var decoded azulLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("promotion: promo service returned status %d with an unreadable body: %w",
			resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || decoded.CustomerKey == "" {
		// A login that failed is an outcome to report to the operator, not a 500. Wrap the
		// service's own message so the panel can show why (bad password, edge block, ...).
		if decoded.Error != "" {
			return "", fmt.Errorf("%w: %s", ErrAzulLoginFailed, decoded.Error)
		}
		return "", ErrAzulLoginFailed
	}
	return decoded.CustomerKey, nil
}
