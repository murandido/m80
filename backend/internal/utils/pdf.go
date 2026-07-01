package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
)

func GeneratePDF(htmlContent string) ([]byte, error) {
	serviceURL := os.Getenv("PDF_SERVICE_URL")
	if serviceURL == "" {
		serviceURL = "http://pdf-service-dev:5000"
	}

	resp, err := http.Post(serviceURL+"/pdf", "text/html", bytes.NewBufferString(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("pdf service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pdf service returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
