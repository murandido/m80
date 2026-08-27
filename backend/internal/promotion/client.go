package promotion

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Recipe is the wire form of a per-site override sent to the promo service. An empty
// field means "resolve this piece heuristically".
type Recipe struct {
	CPFSelector      string `json:"cpf_selector"`
	CheckboxSelector string `json:"checkbox_selector"`
	SubmitSelector   string `json:"submit_selector"`
	SuccessPattern   string `json:"success_pattern"`
	FailurePattern   string `json:"failure_pattern"`
	WaitSeconds      int    `json:"wait_seconds"`
}

type ServiceRequest struct {
	URL    string   `json:"url"`
	CPFs   []string `json:"cpfs"`
	DryRun bool     `json:"dry_run"`
	Recipe *Recipe  `json:"recipe,omitempty"`
}

// Resolution mirrors what the service decided to interact with.
type Resolution struct {
	CPFField string `json:"cpf_field"`
	Checkbox bool   `json:"checkbox"`
	Submit   string `json:"submit"`
	Source   string `json:"source"`
}

type ServiceResult struct {
	CPF        string `json:"cpf"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	Screenshot string `json:"screenshot_base64"`
}

type ServiceResponse struct {
	Resolved   *Resolution     `json:"resolved"`
	Results    []ServiceResult `json:"results"`
	Error      string          `json:"error"`
	Screenshot string          `json:"screenshot_base64"`
}

// Client talks to the promo service over the internal network.
type Client struct {
	baseURL string
	http    *http.Client
}

const (
	defaultServiceURL = "http://promo-service-dev:8082"
	// A browser run is slow by nature: the proof-of-concept measured about twelve
	// seconds per CPF, so a list of ten is two minutes of legitimate work. This is
	// deliberately not utils.GeneratePDF's approach - that one uses http.DefaultClient
	// with no timeout at all, which is fine for WeasyPrint returning in milliseconds
	// and would hang a bot worker forever here.
	defaultTimeout = 30 * time.Minute
)

func NewClient() *Client {
	baseURL := os.Getenv("PROMO_SERVICE_URL")
	if baseURL == "" {
		baseURL = defaultServiceURL
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// Enroll posts one campaign run to the service.
func (c *Client) Enroll(ctx context.Context, req ServiceRequest) (ServiceResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ServiceResponse{}, fmt.Errorf("promotion: encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/enroll", bytes.NewReader(body))
	if err != nil {
		return ServiceResponse{}, fmt.Errorf("promotion: building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ServiceResponse{}, fmt.Errorf("promotion: promo service unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var decoded ServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ServiceResponse{}, fmt.Errorf("promotion: promo service returned status %d with an unreadable body: %w",
			resp.StatusCode, err)
	}
	// A refusal is a 200 carrying an Error field: the service could not find a form and
	// declined to guess. That is an outcome to report, not a transport failure. Any
	// other non-2xx is genuinely broken.
	if resp.StatusCode != http.StatusOK {
		if decoded.Error != "" {
			return decoded, fmt.Errorf("promotion: promo service returned status %d: %s", resp.StatusCode, decoded.Error)
		}
		return decoded, fmt.Errorf("promotion: promo service returned status %d", resp.StatusCode)
	}
	return decoded, nil
}

// DecodeScreenshot turns the service's base64 payload into bytes for WhatsApp. A
// screenshot that will not decode is not worth failing an otherwise good enrolment
// over, so the caller gets nil and carries on.
func DecodeScreenshot(encoded string) []byte {
	if encoded == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	return decoded
}
