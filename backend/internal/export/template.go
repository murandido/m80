package export

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// The MODELO spreadsheet is intentionally not versioned (see root .gitignore). It is
// provisioned on demand: read from disk when present, otherwise downloaded once from the
// shared Google Sheets export and cached at issuanceTemplatePath for subsequent exports.
const (
	// Relative to the API process working directory (backend/ in dev, the container WORKDIR
	// in Docker). Kept in sync with the ignore rule in the root .gitignore.
	issuanceTemplatePath = "assets/templates/emissoes-gerais-modelo.xlsx"
	issuanceTemplateURL  = "https://docs.google.com/spreadsheets/d/1uCKM6yT2xNri6ZPXL3Z2jQRXpgKmyJHe/export?format=xlsx"
)

// loadIssuanceTemplate returns the raw bytes of the MODELO spreadsheet, downloading and
// caching it from Google Sheets the first time it is missing from disk.
func loadIssuanceTemplate() ([]byte, error) {
	if content, err := os.ReadFile(issuanceTemplatePath); err == nil {
		return content, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("loadIssuanceTemplate: read %q: %w", issuanceTemplatePath, err)
	}

	content, err := downloadIssuanceTemplate()
	if err != nil {
		return nil, err
	}

	// Cache best-effort: a write failure (e.g. read-only volume) must not fail the export,
	// since we already hold a valid template in memory. Just fall back to re-downloading.
	if err := os.MkdirAll(filepath.Dir(issuanceTemplatePath), 0o750); err == nil {
		_ = os.WriteFile(issuanceTemplatePath, content, 0o600)
	}

	return content, nil
}

func downloadIssuanceTemplate() ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodGet, issuanceTemplateURL, nil)
	if err != nil {
		return nil, fmt.Errorf("downloadIssuanceTemplate: build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloadIssuanceTemplate: get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloadIssuanceTemplate: unexpected status %d from template source", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("downloadIssuanceTemplate: read body: %w", err)
	}

	return content, nil
}
