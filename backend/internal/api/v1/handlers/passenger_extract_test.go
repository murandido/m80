package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/passengerextract"
)

// These tests need no database: the handler never touches one, and the extractor is a fake,
// so nothing here reaches the paid API.

type fakePassengerExtractor struct {
	got       *passengerextract.Request
	passenger []passengerextract.Passenger
	err       error
}

func (f *fakePassengerExtractor) Extract(_ context.Context, request passengerextract.Request) ([]passengerextract.Passenger, passengerextract.Usage, error) {
	f.got = &request
	return f.passenger, passengerextract.Usage{}, f.err
}

func tinyJPEG(t *testing.T) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func callExtract(handler *PassengerExtractHandler, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/passengers/extract", bytes.NewReader(raw))
	recorder := httptest.NewRecorder()
	handler.ExtractPassengers(recorder, request)
	return recorder
}

func TestExtractPassengers_OffWithoutAKey(t *testing.T) {
	recorder := callExtract(NewPassengerExtractHandler(), map[string]any{"text": "x"})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without an extractor, got %d", recorder.Code)
	}
}

func TestExtractPassengers_SendsDecodedImagesAndText(t *testing.T) {
	name := "Ana"
	fake := &fakePassengerExtractor{passenger: []passengerextract.Passenger{{FirstName: &name}}}
	handler := NewPassengerExtractHandler()
	handler.Extractor = fake

	recorder := callExtract(handler, map[string]any{
		"images": []string{"data:image/jpeg;base64," + tinyJPEG(t), tinyJPEG(t)},
		"text":   "Ana, CPF 529.982.247-25",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body)
	}
	if len(fake.got.Images) != 2 || fake.got.Images[0].MediaType != "image/jpeg" || fake.got.Text == "" {
		t.Fatalf("groups not passed through: %+v", fake.got)
	}
	if !strings.Contains(recorder.Body.String(), `"first_name":"Ana"`) {
		t.Fatalf("passenger missing from the answer: %s", recorder.Body)
	}
}

func TestExtractPassengers_RejectsBadInputBeforeCallingTheModel(t *testing.T) {
	twentyOne := make([]string, 21)
	for index := range twentyOne {
		twentyOne[index] = tinyJPEG(t)
	}
	cases := map[string]map[string]any{
		"nothing":         {"text": "  "},
		"not base64":      {"images": []string{"%%%"}},
		"not an image":    {"images": []string{base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 hello"))}},
		"too many images": {"images": twentyOne},
		"too much text":   {"text": strings.Repeat("a", 20001)},
	}
	for name, body := range cases {
		fake := &fakePassengerExtractor{}
		handler := NewPassengerExtractHandler()
		handler.Extractor = fake
		if recorder := callExtract(handler, body); recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", name, recorder.Code)
		}
		if fake.got != nil {
			t.Errorf("%s: the model must not be called", name)
		}
	}
}

func TestExtractPassengers_MapsExtractorErrors(t *testing.T) {
	cases := map[int]error{
		http.StatusUnprocessableEntity: passengerextract.ErrUnreadable,
		http.StatusBadGateway:          errors.New("connection reset"),
	}
	for want, err := range cases {
		handler := NewPassengerExtractHandler()
		handler.Extractor = &fakePassengerExtractor{err: err}
		if recorder := callExtract(handler, map[string]any{"text": "x"}); recorder.Code != want {
			t.Errorf("%v: expected %d, got %d", err, want, recorder.Code)
		}
	}
}
