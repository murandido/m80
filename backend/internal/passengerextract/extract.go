// Package passengerextract reads passenger data out of whatever staff paste into the browser
// extension - free-form messages and photos of identity documents, mixed, in any amount - so
// the extension can fill an airline's passenger form. Nothing is stored: the input goes to the
// model, the passengers come back, and that is all.
//
// It works in two steps, because one step does not: asked to sort fifteen document photos in a
// single read, the model stopped pairing the front of an ID with its back.
//
//  1. Transcribe: every image is read on its own, in parallel, into what is printed on it. A
//     read that fails a check (CPF digits, impossible dates, a field it could not make out) is
//     read once more by a stronger model - only that image, so the expensive model is paid for
//     rarely (a benchmark on real documents had the cheap one right on 12 of 13).
//  2. Assemble: one text-only call gets those transcriptions plus the pasted text and decides
//     who is who - which images are the same person, which people the message talks about.
//     Reasoning over a few lines of structured data is easy and cheap; reasoning over fifteen
//     photos at once is neither.
//
// Everything that can be decided by rule is decided here, in code, where it is deterministic
// and free: how a name splits into first and last name, the group's shared contact, and
// whether a CPF or a date is even possible.
package passengerextract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/murandido/m-80-backend/internal/utils"
)

// Request is everything pasted at once: any text, and the images in the order they came (order
// helps the assembly pair a back with the front pasted next to it).
type Request struct {
	Text   string
	Images []Image
}

type Image struct {
	MediaType string
	Data      []byte
}

// Passenger is one person as the extension shows and fills it.
type Passenger struct {
	FirstName              *string `json:"first_name"`
	LastName               *string `json:"last_name"`
	BirthDate              *string `json:"birth_date"`
	Gender                 *string `json:"gender"`
	GenderSource           *string `json:"gender_source"`
	Nationality            *string `json:"nationality"`
	CPF                    *string `json:"cpf"`
	DocumentType           *string `json:"document_type"`
	DocumentNumber         *string `json:"document_number"`
	DocumentIssuingCountry *string `json:"document_issuing_country"`
	DocumentExpiry         *string `json:"document_expiry"`
	Email                  *string `json:"email"`
	Phone                  *string `json:"phone"`
	Notes                  *string `json:"notes"`
	// 1-based numbers of the pasted images this passenger was read from; empty when they came
	// from the text alone.
	SourceImages []int `json:"source_images"`
	// Fields a document showed but could not be read with certainty; left for a human.
	IllegibleFields []string `json:"illegible_fields"`
	// Fields filled from another passenger of the same paste (e-mail, phone).
	SharedFields []string `json:"shared_fields"`
	// Problems the checks found, for the extension to flag.
	Warnings []string `json:"warnings"`
	// Whether a stronger model had to re-read one of this passenger's images.
	Escalated bool `json:"escalated"`
}

// Usage is what a request cost, for the log line - never the content.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
	Escalations  int
}

func (u *Usage) add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.Escalations += other.Escalations
}

// Prompt is one call to the model: instructions, content and the JSON schema to answer in.
type Prompt struct {
	System string
	Text   string
	Images []Image
	Schema map[string]any
}

// Caller sends one prompt to one model and returns the raw JSON answer. The real one talks to
// the Claude API (claude.go); tests pass a fake.
type Caller interface {
	Call(ctx context.Context, model string, prompt Prompt) (answer string, usage Usage, err error)
}

// ErrUnreadable means the model answered with something that is not the agreed JSON, or
// declined to answer.
var ErrUnreadable = errors.New("the model did not return passengers")

// ErrEmpty means there was nothing to read.
var ErrEmpty = errors.New("nothing to read")

type Extractor struct {
	Caller Caller
	Model  string
	// EscalationModel re-reads an image whose transcription fails the checks. Empty turns the
	// retry off.
	EscalationModel string
	// How many images are read at once.
	Concurrency int
	// Now is the clock the date checks use; tests pin it.
	Now func() time.Time
}

const callTimeout = 90 * time.Second

// Extract transcribes every image, then assembles the passengers from those transcriptions and
// the text.
func (e *Extractor) Extract(ctx context.Context, request Request) ([]Passenger, Usage, error) {
	if strings.TrimSpace(request.Text) == "" && len(request.Images) == 0 {
		return nil, Usage{}, ErrEmpty
	}
	var total Usage

	transcripts, escalated, usage, err := e.transcribeAll(ctx, request.Images)
	total.add(usage)
	if err != nil {
		return nil, total, err
	}

	passengers, usage, err := e.assemble(ctx, request.Text, transcripts)
	total.add(usage)
	if err != nil {
		return nil, total, err
	}
	passengers = recoverUnassignedImages(passengers, transcripts, e.now())
	passengers = mergeOrphanBacks(passengers, transcripts)
	for index := range passengers {
		for _, image := range passengers[index].SourceImages {
			if image >= 1 && image <= len(escalated) && escalated[image-1] {
				passengers[index].Escalated = true
			}
		}
	}
	shareContacts(passengers)
	return passengers, total, nil
}

// transcript is what one image says, as the assembly step receives it.
type transcript struct {
	Image     int             `json:"image"`
	Documents []modelDocument `json:"documents"`
}

func (e *Extractor) transcribeAll(ctx context.Context, images []Image) ([]transcript, []bool, Usage, error) {
	concurrency := e.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	type outcome struct {
		documents []modelDocument
		escalated bool
		usage     Usage
		err       error
	}
	outcomes := make([]outcome, len(images))
	slots := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	for index, image := range images {
		wait.Add(1)
		go func() {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			documents, escalated, usage, err := e.transcribe(ctx, image)
			outcomes[index] = outcome{documents, escalated, usage, err}
		}()
	}
	wait.Wait()

	transcripts := make([]transcript, 0, len(images))
	escalated := make([]bool, len(images))
	var total Usage
	for index, result := range outcomes {
		total.add(result.usage)
		if result.err != nil {
			return nil, nil, total, fmt.Errorf("image %d: %w", index+1, result.err)
		}
		escalated[index] = result.escalated
		transcripts = append(transcripts, transcript{Image: index + 1, Documents: result.documents})
	}
	return transcripts, escalated, total, nil
}

// transcribe reads one image, and reads it again with the stronger model when the first read
// fails the checks. The second read wins; if it errors, the first one is kept.
func (e *Extractor) transcribe(ctx context.Context, image Image) ([]modelDocument, bool, Usage, error) {
	documents, usage, err := e.readDocuments(ctx, e.Model, image)
	if err != nil && !errors.Is(err, ErrUnreadable) {
		return nil, false, usage, err
	}
	if e.EscalationModel == "" || (err == nil && !documentsNeedRetry(documents, e.now())) {
		return documents, false, usage, err
	}

	retried, retryUsage, retryErr := e.readDocuments(ctx, e.EscalationModel, image)
	usage.add(retryUsage)
	usage.Escalations++
	if retryErr != nil {
		if err == nil {
			return documents, false, usage, nil
		}
		return nil, false, usage, retryErr
	}
	return retried, true, usage, nil
}

func (e *Extractor) readDocuments(ctx context.Context, model string, image Image) ([]modelDocument, Usage, error) {
	raw, usage, err := e.call(ctx, model, Prompt{System: transcribePrompt, Images: []Image{image}, Schema: transcribeSchema()})
	if err != nil {
		return nil, usage, err
	}
	var answer struct {
		Documents []modelDocument `json:"documents"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return nil, usage, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return answer.Documents, usage, nil
}

func (e *Extractor) assemble(ctx context.Context, text string, transcripts []transcript) ([]Passenger, Usage, error) {
	var content strings.Builder
	if strings.TrimSpace(text) != "" {
		content.WriteString("Mensagem colada:\n")
		content.WriteString(text)
		content.WriteString("\n\n")
	}
	if len(transcripts) > 0 {
		encoded, err := json.Marshal(transcripts)
		if err != nil {
			return nil, Usage{}, err
		}
		content.WriteString("Transcrição das imagens de documentos, na ordem em que foram coladas:\n")
		content.Write(encoded)
	}

	raw, usage, err := e.call(ctx, e.Model, Prompt{System: assemblePrompt, Text: content.String(), Schema: passengerSchema()})
	if err != nil {
		return nil, usage, err
	}
	var answer struct {
		Passengers []modelPassenger `json:"passengers"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return nil, usage, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	passengers := make([]Passenger, 0, len(answer.Passengers))
	for _, read := range answer.Passengers {
		passengers = append(passengers, fromModel(read, e.now()))
	}
	return passengers, usage, nil
}

func (e *Extractor) call(ctx context.Context, model string, prompt Prompt) (string, Usage, error) {
	callContext, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return e.Caller.Call(callContext, model, prompt)
}

func (e *Extractor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// recoverUnassignedImages makes sure no pasted image is dropped in silence. The assembly is
// told to use every image, but in the benchmark it once left out the back of an RG - the side
// that held the passenger's CPF. Each image no passenger mentions becomes a passenger of its own,
// read straight from its transcription and flagged; mergeOrphanBacks then folds the ones that
// are only a back into the front next to them.
func recoverUnassignedImages(passengers []Passenger, transcripts []transcript, now time.Time) []Passenger {
	used := map[int]bool{}
	for _, passenger := range passengers {
		for _, image := range passenger.SourceImages {
			used[image] = true
		}
	}
	for _, transcript := range transcripts {
		if used[transcript.Image] {
			continue
		}
		for _, document := range transcript.Documents {
			var genderSource *string
			if document.Gender != nil {
				printed := "printed"
				genderSource = &printed
			}
			passenger := fromModel(modelPassenger{
				FullName:               document.HolderName,
				BirthDate:              document.BirthDate,
				Gender:                 document.Gender,
				GenderSource:           genderSource,
				Nationality:            document.Nationality,
				CPF:                    document.CPF,
				DocumentType:           document.DocumentType,
				DocumentNumber:         document.DocumentNumber,
				DocumentIssuingCountry: document.IssuingCountry,
				DocumentExpiry:         document.ExpiryDate,
				SourceImages:           []int{transcript.Image},
				IllegibleFields:        document.IllegibleFields,
			}, now)
			passenger.Warnings = append(passenger.Warnings, "unassembled")
			passengers = append(passengers, passenger)
		}
	}
	return passengers
}

// mergeOrphanBacks folds a "passenger" read only from the back of a document into the
// passenger holding the front pasted next to it. The assembly is told to do this, and usually
// does, but not always: in the benchmark the back of an RG became a passenger of its own - once
// under the father's name, taken from the filiação, and once with no name at all, leaving the
// real passenger without the CPF that only the back carried. A back on its own is never a
// person, so this does not depend on the model getting it right.
//
// The front pasted just before wins over the one just after (people paste front, then back).
// The document type is not compared: the transcription has labelled the two sides of the same
// CIN once as CIN and once as RG. The orphan only fills what the front's passenger lacks; its
// name is dropped, since a back carries no holder name worth trusting.
func mergeOrphanBacks(passengers []Passenger, transcripts []transcript) []Passenger {
	documentsOf := func(image int) []modelDocument {
		if image < 1 || image > len(transcripts) {
			return nil
		}
		return transcripts[image-1].Documents
	}
	onlyBacks := func(passenger Passenger) bool {
		if len(passenger.SourceImages) == 0 {
			return false
		}
		for _, image := range passenger.SourceImages {
			documents := documentsOf(image)
			if len(documents) == 0 {
				return false
			}
			for _, document := range documents {
				if document.Side == nil || *document.Side != "back" {
					return false
				}
			}
		}
		return true
	}
	hasFront := func(image int) bool {
		for _, document := range documentsOf(image) {
			if document.Side == nil || *document.Side != "back" {
				return true
			}
		}
		return false
	}
	owner := func(image, except int) int {
		for index, passenger := range passengers {
			if index != except && slices.Contains(passenger.SourceImages, image) {
				return index
			}
		}
		return -1
	}

	merged := make([]bool, len(passengers))
	for index, passenger := range passengers {
		if !onlyBacks(passenger) {
			continue
		}
		first := slices.Min(passenger.SourceImages)
		last := slices.Max(passenger.SourceImages)
		target := -1
		for _, neighbour := range []int{first - 1, last + 1} {
			if candidate := owner(neighbour, index); candidate >= 0 && !merged[candidate] && hasFront(neighbour) {
				target = candidate
				break
			}
		}
		if target < 0 {
			continue
		}
		fillMissing(&passengers[target], passenger)
		merged[index] = true
	}

	kept := make([]Passenger, 0, len(passengers))
	for index, passenger := range passengers {
		if !merged[index] {
			kept = append(kept, passenger)
		}
	}
	return kept
}

// fillMissing completes target with what orphan read and target did not, and takes its images.
func fillMissing(target *Passenger, orphan Passenger) {
	for _, pair := range [][2]**string{
		{&target.BirthDate, &orphan.BirthDate},
		{&target.CPF, &orphan.CPF},
		{&target.Nationality, &orphan.Nationality},
		{&target.DocumentType, &orphan.DocumentType},
		{&target.DocumentNumber, &orphan.DocumentNumber},
		{&target.DocumentIssuingCountry, &orphan.DocumentIssuingCountry},
		{&target.DocumentExpiry, &orphan.DocumentExpiry},
		{&target.Email, &orphan.Email},
		{&target.Phone, &orphan.Phone},
	} {
		if *pair[0] == nil {
			*pair[0] = *pair[1]
		}
	}
	target.SourceImages = append(target.SourceImages, orphan.SourceImages...)
	slices.Sort(target.SourceImages)
	for _, field := range orphan.IllegibleFields {
		if !slices.Contains(target.IllegibleFields, field) {
			target.IllegibleFields = append(target.IllegibleFields, field)
		}
	}
	for _, warning := range orphan.Warnings {
		if warning != "unassembled" && !slices.Contains(target.Warnings, warning) {
			target.Warnings = append(target.Warnings, warning)
		}
	}
}

// modelDocument is what one document (or one side of it) says, as transcribed.
type modelDocument struct {
	Side            *string  `json:"side"`
	HolderName      *string  `json:"holder_name"`
	MotherName      *string  `json:"mother_name"`
	FatherName      *string  `json:"father_name"`
	BirthDate       *string  `json:"birth_date"`
	Gender          *string  `json:"gender"`
	Nationality     *string  `json:"nationality"`
	CPF             *string  `json:"cpf"`
	DocumentType    *string  `json:"document_type"`
	DocumentNumber  *string  `json:"document_number"`
	RGNumber        *string  `json:"rg_number"`
	IssuingCountry  *string  `json:"issuing_country"`
	IssueDate       *string  `json:"issue_date"`
	ExpiryDate      *string  `json:"expiry_date"`
	IllegibleFields []string `json:"illegible_fields"`
}

// documentsNeedRetry is true when a transcription found nothing, could not read a field it
// saw, or read something that cannot be right.
func documentsNeedRetry(documents []modelDocument, now time.Time) bool {
	if len(documents) == 0 {
		return true
	}
	for _, document := range documents {
		if len(document.IllegibleFields) > 0 {
			return true
		}
		if cpf := clean(document.CPF); cpf != nil {
			if _, ok := utils.NormalizeCPF(*cpf); !ok {
				return true
			}
		}
		if !plausibleBirthDate(document.BirthDate, now) || !validDate(document.ExpiryDate) || !validDate(document.IssueDate) {
			return true
		}
	}
	return false
}

// modelPassenger is the shape the assembly answers in. The name comes whole: splitting it is a
// rule of the airline forms, not something to ask the model to guess.
type modelPassenger struct {
	FullName               *string  `json:"full_name"`
	BirthDate              *string  `json:"birth_date"`
	Gender                 *string  `json:"gender"`
	GenderSource           *string  `json:"gender_source"`
	Nationality            *string  `json:"nationality"`
	CPF                    *string  `json:"cpf"`
	DocumentType           *string  `json:"document_type"`
	DocumentNumber         *string  `json:"document_number"`
	DocumentIssuingCountry *string  `json:"document_issuing_country"`
	DocumentExpiry         *string  `json:"document_expiry"`
	Email                  *string  `json:"email"`
	Phone                  *string  `json:"phone"`
	SourceImages           []int    `json:"source_images"`
	IllegibleFields        []string `json:"illegible_fields"`
	Notes                  *string  `json:"notes"`
}

func fromModel(read modelPassenger, now time.Time) Passenger {
	first, last := SplitName(read.FullName)
	passenger := Passenger{
		FirstName:              first,
		LastName:               last,
		BirthDate:              clean(read.BirthDate),
		Gender:                 normalizeGender(read.Gender),
		GenderSource:           clean(read.GenderSource),
		Nationality:            upper(read.Nationality),
		DocumentType:           upper(read.DocumentType),
		DocumentNumber:         clean(read.DocumentNumber),
		DocumentIssuingCountry: upper(read.DocumentIssuingCountry),
		DocumentExpiry:         clean(read.DocumentExpiry),
		Email:                  clean(read.Email),
		Phone:                  digits(read.Phone),
		Notes:                  clean(read.Notes),
		SourceImages:           nonNilInts(read.SourceImages),
		IllegibleFields:        nonNil(read.IllegibleFields),
		SharedFields:           []string{},
		Warnings:               []string{},
	}

	if cpf := clean(read.CPF); cpf != nil {
		if normalized, ok := utils.NormalizeCPF(*cpf); ok {
			passenger.CPF = &normalized
		} else {
			// Kept as read, so the employee sees what the document seemed to say.
			passenger.CPF = cpf
			passenger.Warnings = append(passenger.Warnings, "cpf_invalid")
		}
	}
	if !plausibleBirthDate(passenger.BirthDate, now) {
		passenger.Warnings = append(passenger.Warnings, "birth_date_invalid")
	}
	if !validDate(passenger.DocumentExpiry) {
		passenger.Warnings = append(passenger.Warnings, "document_expiry_invalid")
	}
	return passenger
}

func validDate(field *string) bool {
	cleaned := clean(field)
	if cleaned == nil {
		return true
	}
	_, err := time.Parse(time.DateOnly, *cleaned)
	return err == nil
}

func plausibleBirthDate(field *string, now time.Time) bool {
	cleaned := clean(field)
	if cleaned == nil {
		return true
	}
	birth, err := time.Parse(time.DateOnly, *cleaned)
	return err == nil && !birth.After(now) && birth.Year() >= 1900
}

// SplitName applies the forms' convention: the first word is the first name, everything
// after it the last name ("Antonio Werbson da Silva" -> "Antonio" / "Werbson da Silva").
func SplitName(full *string) (*string, *string) {
	if full == nil {
		return nil, nil
	}
	words := strings.Fields(*full)
	if len(words) == 0 {
		return nil, nil
	}
	first := words[0]
	if len(words) == 1 {
		return &first, nil
	}
	last := strings.Join(words[1:], " ")
	return &first, &last
}

// shareContacts copies the first e-mail and phone of the paste to every passenger without
// their own: a booking usually carries one contact for the whole group, and the airline forms
// want one per passenger. Someone with their own keeps it.
func shareContacts(passengers []Passenger) {
	var email, phone *string
	for _, passenger := range passengers {
		if email == nil && passenger.Email != nil {
			email = passenger.Email
		}
		if phone == nil && passenger.Phone != nil {
			phone = passenger.Phone
		}
	}
	for index := range passengers {
		passenger := &passengers[index]
		if passenger.Email == nil && email != nil {
			copied := *email
			passenger.Email = &copied
			passenger.SharedFields = append(passenger.SharedFields, "email")
		}
		if passenger.Phone == nil && phone != nil {
			copied := *phone
			passenger.Phone = &copied
			passenger.SharedFields = append(passenger.SharedFields, "phone")
		}
	}
}

func clean(field *string) *string {
	if field == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*field)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func upper(field *string) *string {
	cleaned := clean(field)
	if cleaned == nil {
		return nil
	}
	upperCased := strings.ToUpper(*cleaned)
	return &upperCased
}

func digits(field *string) *string {
	cleaned := clean(field)
	if cleaned == nil {
		return nil
	}
	only := strings.Map(func(character rune) rune {
		if character >= '0' && character <= '9' {
			return character
		}
		return -1
	}, *cleaned)
	if only == "" {
		return nil
	}
	return &only
}

func normalizeGender(field *string) *string {
	cleaned := upper(field)
	if cleaned == nil || (*cleaned != "M" && *cleaned != "F") {
		return nil
	}
	return cleaned
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilInts(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}
