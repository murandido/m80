package passengerextract

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCaller answers the per-image read and the assembly separately, per model, and keeps
// what it was asked so a test can look at it.
type fakeCaller struct {
	mu         sync.Mutex
	transcribe map[string]func(image Image) (string, error)
	assemble   func(text string) (string, error)
	calls      map[string]int
	assembled  string
}

func (f *fakeCaller) Call(_ context.Context, model string, prompt Prompt) (string, Usage, error) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[model]++
	f.mu.Unlock()
	usage := Usage{InputTokens: 1000, OutputTokens: 100}
	if prompt.System == assemblePrompt {
		f.mu.Lock()
		f.assembled = prompt.Text
		f.mu.Unlock()
		answer, err := f.assemble(prompt.Text)
		return answer, usage, err
	}
	answer, err := f.transcribe[model](prompt.Images[0])
	return answer, usage, err
}

func toJSON(t *testing.T, key string, records ...map[string]any) string {
	t.Helper()
	if records == nil {
		records = []map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{key: records})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var fixedNow = func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) }

func newExtractor(caller Caller) *Extractor {
	return &Extractor{Caller: caller, Model: "cheap", EscalationModel: "strong", Now: fixedNow}
}

func image(tag string) Image { return Image{MediaType: "image/jpeg", Data: []byte(tag)} }

func TestSplitNameKeepsTheFirstWordAsFirstName(t *testing.T) {
	cases := map[string][2]string{
		"Antonio Werbson da Silva":       {"Antonio", "Werbson da Silva"},
		"  MARIA   APARECIDA  ANDERSON ": {"MARIA", "APARECIDA ANDERSON"},
	}
	for full, want := range cases {
		first, last := SplitName(&full)
		if *first != want[0] || *last != want[1] {
			t.Errorf("%q -> %q / %q", full, *first, *last)
		}
	}
	single := "Pelé"
	if first, last := SplitName(&single); *first != "Pelé" || last != nil {
		t.Errorf("single word should have no last name")
	}
}

func TestEveryImageIsTranscribedAndTheAssemblySeesThemInOrder(t *testing.T) {
	caller := &fakeCaller{
		transcribe: map[string]func(Image) (string, error){
			"cheap": func(image Image) (string, error) {
				return toJSON(t, "documents", map[string]any{"side": "front", "holder_name": "doc " + string(image.Data)}), nil
			},
		},
		assemble: func(string) (string, error) {
			return toJSON(t, "passengers", map[string]any{
				"full_name": "Ana Paula da Silva Rocha", "cpf": "000.123.456-01", "gender": "f",
				"gender_source": "printed", "source_images": []int{1, 2},
			}), nil
		},
	}
	passengers, usage, err := newExtractor(caller).Extract(context.Background(), Request{
		Text:   "Ana, e-mail ana@x.com",
		Images: []Image{image("front"), image("back")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls["cheap"] != 3 || caller.calls["strong"] != 0 || usage.Escalations != 0 {
		t.Fatalf("expected two reads and one assembly, no escalation: %v", caller.calls)
	}
	if !strings.Contains(caller.assembled, "Ana, e-mail") || strings.Index(caller.assembled, "doc front") > strings.Index(caller.assembled, "doc back") {
		t.Fatalf("the assembly needs the text and the images in order: %s", caller.assembled)
	}
	got := passengers[0]
	if *got.FirstName != "Ana" || *got.LastName != "Paula da Silva Rocha" || *got.CPF != "00012345601" || *got.Gender != "F" {
		t.Fatalf("unexpected passenger: %+v", got)
	}
	if len(got.SourceImages) != 2 || got.Escalated {
		t.Fatalf("source images and escalation: %+v", got)
	}
}

func TestOnlyTheImageThatFailsIsReadAgainByTheStrongerModel(t *testing.T) {
	caller := &fakeCaller{
		transcribe: map[string]func(Image) (string, error){
			"cheap": func(image Image) (string, error) {
				cpf := "52998224725"
				if string(image.Data) == "blurry" {
					cpf = "00012345603"
				}
				return toJSON(t, "documents", map[string]any{"holder_name": "X", "cpf": cpf}), nil
			},
			"strong": func(Image) (string, error) {
				return toJSON(t, "documents", map[string]any{"holder_name": "X", "cpf": "00012345601"}), nil
			},
		},
		assemble: func(text string) (string, error) {
			if !strings.Contains(text, "00012345601") || strings.Contains(text, "00012345603") {
				return "", errors.New("the assembly must see the re-read CPF only")
			}
			return toJSON(t, "passengers",
				map[string]any{"full_name": "A", "source_images": []int{1}},
				map[string]any{"full_name": "B", "source_images": []int{2}},
			), nil
		},
	}
	passengers, usage, err := newExtractor(caller).Extract(context.Background(), Request{Images: []Image{image("sharp"), image("blurry")}})
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls["strong"] != 1 || usage.Escalations != 1 {
		t.Fatalf("only the blurry image is re-read: %v", caller.calls)
	}
	if passengers[0].Escalated || !passengers[1].Escalated {
		t.Fatalf("escalation follows the source image: %+v", passengers)
	}
}

func TestTheFirstReadSurvivesAFailedRetry(t *testing.T) {
	caller := &fakeCaller{
		transcribe: map[string]func(Image) (string, error){
			"cheap": func(Image) (string, error) {
				return toJSON(t, "documents", map[string]any{"holder_name": "X", "illegible_fields": []string{"cpf"}}), nil
			},
			"strong": func(Image) (string, error) { return "", errors.New("overloaded") },
		},
		assemble: func(text string) (string, error) {
			return toJSON(t, "passengers", map[string]any{"full_name": "X", "source_images": []int{1}, "illegible_fields": []string{"cpf"}}), nil
		},
	}
	passengers, _, err := newExtractor(caller).Extract(context.Background(), Request{Images: []Image{image("a")}})
	if err != nil || len(passengers) != 1 || passengers[0].Escalated || passengers[0].IllegibleFields[0] != "cpf" {
		t.Fatalf("expected the flagged first read: %v %+v", err, passengers)
	}
}

func TestTextAloneSkipsTheTranscription(t *testing.T) {
	caller := &fakeCaller{assemble: func(string) (string, error) {
		return toJSON(t, "passengers",
			map[string]any{"full_name": "A", "email": "grupo@x.com"},
			map[string]any{"full_name": "B", "phone": "+55 (11) 98888-7777"},
			map[string]any{"full_name": "C", "email": "proprio@x.com"},
		), nil
	}}
	passengers, _, err := newExtractor(caller).Extract(context.Background(), Request{Text: "três pessoas"})
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls["cheap"] != 1 {
		t.Fatalf("text only is one call: %v", caller.calls)
	}
	if *passengers[1].Email != "grupo@x.com" || passengers[1].SharedFields[0] != "email" {
		t.Fatalf("second should get the group e-mail: %+v", passengers[1])
	}
	if *passengers[2].Email != "proprio@x.com" || *passengers[2].Phone != "5511988887777" {
		t.Fatalf("third keeps its own e-mail and gets the phone: %+v", passengers[2])
	}
	if *passengers[0].Phone != "5511988887777" {
		t.Fatal("the first gets the phone that only the second had")
	}
}

func TestChecksFlagWhatTheAssemblyReturns(t *testing.T) {
	caller := &fakeCaller{assemble: func(string) (string, error) {
		return toJSON(t, "passengers", map[string]any{"full_name": "X Y", "cpf": "12345678900", "birth_date": "2030-01-01"}), nil
	}}
	passengers, _, err := newExtractor(caller).Extract(context.Background(), Request{Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	warnings := passengers[0].Warnings
	if len(warnings) != 2 || warnings[0] != "cpf_invalid" || warnings[1] != "birth_date_invalid" {
		t.Fatalf("warnings: %v", warnings)
	}
	if *passengers[0].CPF != "12345678900" {
		t.Fatal("an invalid CPF is kept as read, for the employee to compare")
	}
}

func TestEmptyAndUnreadable(t *testing.T) {
	extractor := newExtractor(&fakeCaller{assemble: func(string) (string, error) { return "not json", nil }})
	if _, _, err := extractor.Extract(context.Background(), Request{Text: "  "}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
	if _, _, err := extractor.Extract(context.Background(), Request{Text: "x"}); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("expected ErrUnreadable, got %v", err)
	}
}

func TestBothSchemasRequireEveryField(t *testing.T) {
	for name, schema := range map[string]map[string]any{"documents": transcribeSchema(), "passengers": passengerSchema()} {
		items := schema["properties"].(map[string]any)[name].(map[string]any)["items"].(map[string]any)
		if len(items["required"].([]string)) != len(items["properties"].(map[string]any)) {
			t.Fatalf("%s: strict structured outputs need every property in required", name)
		}
	}
}

func TestABackReadAsAPassengerIsFoldedIntoItsFront(t *testing.T) {
	side := func(value string) *string { return &value }
	rg := "RG"
	cin := "CIN"
	transcripts := []transcript{
		{Image: 1, Documents: []modelDocument{{Side: side("front"), DocumentType: &cin}}},
		// The back of the same CIN, labelled RG by the transcription: the type must not matter.
		{Image: 2, Documents: []modelDocument{{Side: side("back"), DocumentType: &rg}}},
		{Image: 3, Documents: []modelDocument{{Side: side("front"), DocumentType: &rg}}},
		{Image: 4, Documents: []modelDocument{{Side: side("back"), DocumentType: &rg}}},
		{Image: 5, Documents: []modelDocument{{Side: side("single")}}},
	}
	name := func(value string) *string { return &value }
	cpf := "28144579852"
	passengers := []Passenger{
		{FirstName: name("MANOEL"), SourceImages: []int{1}},
		// The father's name, lifted from the back's filiação.
		{FirstName: name("FRACOLINO"), SourceImages: []int{2}},
		{FirstName: name("MARIA"), SourceImages: []int{3}},
		{CPF: &cpf, SourceImages: []int{4}, IllegibleFields: []string{"rg_number"}},
		{FirstName: name("PEDRO"), SourceImages: []int{5}},
	}

	got := mergeOrphanBacks(passengers, transcripts)

	if len(got) != 3 {
		t.Fatalf("expected the two backs folded in, got %d passengers", len(got))
	}
	if *got[0].FirstName != "MANOEL" || len(got[0].SourceImages) != 2 {
		t.Fatalf("Manoel keeps his name and takes the back: %+v", got[0])
	}
	if *got[1].FirstName != "MARIA" || got[1].CPF == nil || *got[1].CPF != cpf || got[1].IllegibleFields[0] != "rg_number" {
		t.Fatalf("Maria gets the CPF only the back carried: %+v", got[1])
	}
}

func TestABackWithNoFrontNextToItStays(t *testing.T) {
	back := "back"
	transcripts := []transcript{{Image: 1, Documents: []modelDocument{{Side: &back}}}}
	passengers := []Passenger{{SourceImages: []int{1}}}
	if got := mergeOrphanBacks(passengers, transcripts); len(got) != 1 {
		t.Fatal("with nothing to fold into, the back is shown as it is for a human to sort out")
	}
}

func TestAnImageTheAssemblyLeftOutIsNotLost(t *testing.T) {
	side := func(value string) *string { return &value }
	cpf := "28144579852"
	name := "JOSE DA SILVA"
	transcripts := []transcript{
		{Image: 1, Documents: []modelDocument{{Side: side("front")}}},
		{Image: 2, Documents: []modelDocument{{Side: side("back"), CPF: &cpf}}},
		{Image: 3, Documents: []modelDocument{{Side: side("single"), HolderName: &name}}},
	}
	maria := "MARIA"
	passengers := []Passenger{{FirstName: &maria, SourceImages: []int{1}, Warnings: []string{}}}

	got := mergeOrphanBacks(recoverUnassignedImages(passengers, transcripts, fixedNow()), transcripts)

	if len(got) != 2 {
		t.Fatalf("expected Maria with her back, plus José: %+v", got)
	}
	if got[0].CPF == nil || *got[0].CPF != cpf || len(got[0].SourceImages) != 2 || slices.Contains(got[0].Warnings, "unassembled") {
		t.Fatalf("the left-out back completes Maria, without its flag: %+v", got[0])
	}
	if *got[1].FirstName != "JOSE" || !slices.Contains(got[1].Warnings, "unassembled") {
		t.Fatalf("a left-out front comes back as a flagged passenger: %+v", got[1])
	}
}
