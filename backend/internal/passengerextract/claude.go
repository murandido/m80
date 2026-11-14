package passengerextract

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	defaultModel           = "claude-haiku-5-5"
	defaultEscalationModel = "claude-sonnet-5-5"
	// Low effort read 13 of 13 documents as well as medium did, at a fraction of the output
	// tokens. The output cap leaves room for thinking on a hard photo without truncating.
	effort    = anthropic.OutputConfigEffortLow
	maxTokens = 16000
)

// NewFromEnv builds the extractor from ANTHROPIC_API_KEY, PASSENGER_EXTRACTION_MODEL and
// PASSENGER_EXTRACTION_ESCALATION_MODEL. Without a key it returns nil: the endpoint then
// answers that extraction is off instead of failing on every call. Blank models mean the
// defaults - Compose hands an unset variable over as an empty string, so blank cannot be what
// switches the retry off; "off" does.
func NewFromEnv() *Extractor {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil
	}
	model := os.Getenv("PASSENGER_EXTRACTION_MODEL")
	if model == "" {
		model = defaultModel
	}
	escalation := os.Getenv("PASSENGER_EXTRACTION_ESCALATION_MODEL")
	switch escalation {
	case "":
		escalation = defaultEscalationModel
	case "off":
		escalation = ""
	}
	client := anthropic.NewClient(option.WithAPIKey(key))
	return &Extractor{
		Caller:          &ClaudeCaller{Client: &client},
		Model:           model,
		EscalationModel: escalation,
	}
}

type ClaudeCaller struct {
	Client *anthropic.Client
}

func (c *ClaudeCaller) Call(ctx context.Context, model string, prompt Prompt) (string, Usage, error) {
	message, err := c.Client.Messages.New(ctx, buildRequest(model, prompt))
	if err != nil {
		return "", Usage{}, fmt.Errorf("passengerextract: claude: %w", err)
	}
	usage := Usage{InputTokens: message.Usage.InputTokens, OutputTokens: message.Usage.OutputTokens}

	switch message.StopReason {
	case anthropic.StopReasonRefusal:
		return "", usage, fmt.Errorf("%w: declined", ErrUnreadable)
	case anthropic.StopReasonMaxTokens:
		return "", usage, fmt.Errorf("%w: answer cut at max_tokens", ErrUnreadable)
	}
	var answer strings.Builder
	for _, block := range message.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			answer.WriteString(text.Text)
		}
	}
	return answer.String(), usage, nil
}

func buildRequest(model string, prompt Prompt) anthropic.MessageNewParams {
	var blocks []anthropic.ContentBlockParamUnion
	for _, image := range prompt.Images {
		blocks = append(blocks, anthropic.NewImageBlockBase64(image.MediaType, base64.StdEncoding.EncodeToString(image.Data)))
	}
	if strings.TrimSpace(prompt.Text) != "" {
		blocks = append(blocks, anthropic.NewTextBlock(prompt.Text))
	}
	return anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: prompt.System}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(blocks...)},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: effort,
			Format: anthropic.JSONOutputFormatParam{Schema: prompt.Schema},
		},
	}
}

const transcribePrompt = `Você recebe uma foto enviada por uma agência de viagens brasileira: normalmente um documento de identidade (RG, CIN, CNH, passaporte, RNE) ou um lado dele - frente ou verso. Pode haver mais de um documento na mesma foto.

Para cada documento, ou lado de documento, visível na foto, transcreva o que está impresso:
- holder_name é o nome do titular. Nomes de pai e mãe (filiação) vão em father_name e mother_name, nunca em holder_name. No verso do RG e da CIN aparecem quase só filiação, naturalidade e números: se o nome do titular não estiver impresso nesse lado, holder_name fica null - nunca use um nome da filiação no lugar dele.
- side: front, back, ou single quando o documento inteiro está na mesma face (passaporte, CNH aberta).
- Transcreva exatamente o que está impresso. Nunca deduza nem complete um dígito: se um campo existe mas não dá para ler com certeza, deixe-o null e coloque o nome dele em illegible_fields.
- Campo que não existe no documento fica null e não entra em illegible_fields.
- gender só se estiver impresso.
- Datas em AAAA-MM-DD. CPF só com os 11 dígitos.
- Se a foto não tiver documento nenhum, devolva documents vazio.`

const assemblePrompt = `Você monta a lista de passageiros de uma emissão de passagens de uma agência de viagens brasileira. Recebe uma mensagem colada pelo funcionário (WhatsApp, e-mail, planilha), a transcrição de fotos de documentos, ou os dois.

- Cada pessoa vira um passageiro só. Junte o que for da mesma pessoa: frente e verso do mesmo documento, documentos diferentes dela, e o que a mensagem diz sobre ela.
- Para saber se duas transcrições são da mesma pessoa, use CPF, número do RG, nome, data de nascimento e filiação. Um verso sem o nome do titular geralmente é da frente do mesmo tipo de documento colada logo antes ou logo depois dele.
- Nomes de pai e mãe (filiação) nunca são passageiros, e um verso sozinho nunca é um passageiro: ele completa a frente do mesmo documento.
- Copie os valores exatamente como estão na transcrição ou na mensagem. Nunca invente nem complete dados; o que não houver fica null. Se a mensagem e o documento discordarem, use o documento e explique em notes.
- full_name é o nome completo. document_type/document_number/document_expiry são do documento de viagem: o passaporte quando houver, senão o documento de identidade.
- source_images lista os números das imagens usadas para aquela pessoa (vazio se ela veio só da mensagem). illegible_fields junta os campos ilegíveis dessas imagens.
- Datas em AAAA-MM-DD. CPF só com os 11 dígitos. Telefone só com dígitos, com DDI e DDD quando houver.
- Sexo (gender) sempre preenchido com M ou F. Se estiver impresso ou dito, use-o e gender_source = printed. Se não, deduza pelo primeiro nome: gender_source = inferred quando o nome deixa claro, guess quando o nome serve para os dois sexos (dê o seu melhor palpite).
- Se algo estiver ambíguo, explique em notes, em português.`

func nullableString(description string) map[string]any {
	return map[string]any{"type": []string{"string", "null"}, "description": description}
}

func stringArray(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}

// objectList is the strict structured-output shape both answers use: an object holding one
// array of records, every property listed as required (null is how a field says "not there").
func objectList(name string, properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for property := range properties {
		required = append(required, property)
	}
	sort.Strings(required)
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{name},
		"properties": map[string]any{
			name: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             required,
					"properties":           properties,
				},
			},
		},
	}
}

// transcribeSchema is the answer of the per-image read (modelDocument).
func transcribeSchema() map[string]any {
	return objectList("documents", map[string]any{
		"side":             nullableString("front, back or single."),
		"holder_name":      nullableString("The holder's full name exactly as printed; never a parent's name."),
		"mother_name":      nullableString("Mother's name (filiação), if printed."),
		"father_name":      nullableString("Father's name (filiação), if printed."),
		"birth_date":       nullableString("YYYY-MM-DD."),
		"gender":           nullableString("M or F, only when printed."),
		"nationality":      nullableString("ISO 3166-1 alpha-3, e.g. BRA."),
		"cpf":              nullableString("11 digits, no punctuation."),
		"document_type":    nullableString("One of RG, CIN, CNH, PASSPORT, RNE, OTHER."),
		"document_number":  nullableString("The document's own number as printed (passport number, CNH registro, RG)."),
		"rg_number":        nullableString("The RG/identity number when printed on another document (e.g. CNH field 4c)."),
		"issuing_country":  nullableString("ISO 3166-1 alpha-3."),
		"issue_date":       nullableString("YYYY-MM-DD."),
		"expiry_date":      nullableString("YYYY-MM-DD."),
		"illegible_fields": stringArray("Fields present on the document that could not be read with certainty."),
	})
}

// passengerSchema is the answer of the assembly (modelPassenger).
func passengerSchema() map[string]any {
	return objectList("passengers", map[string]any{
		"full_name":                nullableString("Full name exactly as written."),
		"birth_date":               nullableString("YYYY-MM-DD."),
		"gender":                   nullableString("M or F. Always filled."),
		"gender_source":            nullableString("printed, inferred or guess."),
		"nationality":              nullableString("ISO 3166-1 alpha-3, e.g. BRA."),
		"cpf":                      nullableString("11 digits, no punctuation."),
		"document_type":            nullableString("One of RG, CIN, CNH, PASSPORT, RNE, OTHER."),
		"document_number":          nullableString("As printed, without spaces."),
		"document_issuing_country": nullableString("ISO 3166-1 alpha-3."),
		"document_expiry":          nullableString("YYYY-MM-DD."),
		"email":                    nullableString("Contact e-mail, if given."),
		"phone":                    nullableString("Contact phone, digits only, with country and area code when given."),
		"source_images": map[string]any{
			"type": "array", "items": map[string]any{"type": "integer"},
			"description": "1-based numbers of the images this passenger was read from.",
		},
		"illegible_fields": stringArray("Fields of this passenger's images that could not be read with certainty."),
		"notes":            nullableString("Anything ambiguous a human should check, in Portuguese."),
	})
}
