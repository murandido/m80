package utils

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

func GeneratePDF(htmlContent string) ([]byte, error) {
	serviceURL := os.Getenv("PDF_SERVICE_URL")
	if serviceURL == "" {
		serviceURL = "http://pdf-service-dev:5000"
	}

	resp, err := http.Post(serviceURL+"/pdf", "text/html", bytes.NewBufferString(htmlContent)) // #nosec G704 -- serviceURL vem de PDF_SERVICE_URL (env confiável), não de input de usuário
	if err != nil {
		return nil, fmt.Errorf("pdf service unreachable: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Error("failed to close pdf service response body", "err", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pdf service returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
