package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/passengerextract"
	"github.com/murandido/m-80-backend/internal/utils"
)

const (
	// The extension sends photos already shrunk to ~1568px JPEG, a few hundred KB each; 5 MB
	// is the Claude API's own per-image ceiling.
	maxPassengerImageBytes = 5 << 20
	// Twenty images and the text. Shrunk photos are a few hundred KB, so this is far above any
	// real paste and only stops a runaway body.
	maxPassengerExtractBody = 64 << 20
)

type passengerExtractor interface {
	Extract(ctx context.Context, request passengerextract.Request) ([]passengerextract.Passenger, passengerextract.Usage, error)
}

type PassengerExtractHandler struct {
	// Nil when ANTHROPIC_API_KEY is not set: the route then answers that the feature is off.
	Extractor passengerExtractor
	Validator *validator.Validate
}

func NewPassengerExtractHandler() *PassengerExtractHandler {
	return &PassengerExtractHandler{Validator: validator.New()}
}

// ExtractPassengers handles the POST /passengers/extract route.
// @Summary      Extrair passageiros de texto ou foto de documento
// @Description  Lê com IA os passageiros de uma mensagem colada ou de fotos de documentos, para a extensão preencher o formulário da cia. Nada é guardado.
// @Tags         Passageiros
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload  body  schemas.PassengerExtractRequest  true  "Texto e imagens colados, misturados"
// @Success      200  {array}   passengerextract.Passenger
// @Failure      400  {object}  utils.StandardResponse
// @Failure      422  {object}  utils.StandardResponse
// @Failure      429  {object}  utils.StandardResponse
// @Failure      502  {object}  utils.StandardResponse
// @Failure      503  {object}  utils.StandardResponse
// @Router       /passengers/extract [post]
func (h *PassengerExtractHandler) ExtractPassengers(w http.ResponseWriter, r *http.Request) {
	if h.Extractor == nil {
		utils.SendError(w, r, http.StatusServiceUnavailable, "A leitura de passageiros está desligada neste servidor")
		return
	}

	var req schemas.PassengerExtractRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPassengerExtractBody)).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	request := passengerextract.Request{Text: req.Text}
	for _, encoded := range req.Images {
		image, message := decodePassengerImage(encoded)
		if message != "" {
			utils.SendError(w, r, http.StatusBadRequest, message)
			return
		}
		request.Images = append(request.Images, image)
	}
	if strings.TrimSpace(request.Text) == "" && len(request.Images) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "Cole um texto ou uma imagem de documento")
		return
	}

	passengers, usage, err := h.Extractor.Extract(r.Context(), request)
	user, _ := r.Context().Value(middleware.UserContextKey).(data.User)
	// What it cost and who asked - never what was read.
	slog.Info("passenger extraction", "user_id", user.ID, "images", len(request.Images), "text", request.Text != "",
		"input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens,
		"escalations", usage.Escalations, "ok", err == nil)
	if err != nil {
		if errors.Is(err, passengerextract.ErrUnreadable) {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não consegui ler os passageiros. Tente uma foto mais nítida ou o texto.")
			return
		}
		utils.SendError(w, r, http.StatusBadGateway, "O serviço de leitura não respondeu. Tente de novo.")
		return
	}
	if passengers == nil {
		passengers = []passengerextract.Passenger{}
	}
	utils.SendJSON(w, r, http.StatusOK, "Passengers extracted successfully", passengers)
}

// decodePassengerImage turns one base64 image into bytes and checks it is an image the model
// takes. The type is sniffed from the bytes, not trusted from the caller.
func decodePassengerImage(encoded string) (passengerextract.Image, string) {
	// Accept a data URL as well as bare base64: it is what a canvas hands the extension.
	if comma := strings.IndexByte(encoded, ','); strings.HasPrefix(encoded, "data:") && comma >= 0 {
		encoded = encoded[comma+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return passengerextract.Image{}, "Imagem inválida"
	}
	if len(raw) > maxPassengerImageBytes {
		return passengerextract.Image{}, "Imagem maior que 5 MB"
	}
	mediaType := http.DetectContentType(raw)
	switch mediaType {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return passengerextract.Image{MediaType: mediaType, Data: raw}, ""
	}
	return passengerextract.Image{}, "Formato de imagem não aceito (use JPEG, PNG ou WebP)"
}
