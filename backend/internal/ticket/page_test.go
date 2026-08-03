package ticket

import "testing"

// The numbers asserted here were measured by rendering each shape through the
// real pdf-service (WeasyPrint 61.2) and reading the used content height back
// out of the box tree. They are not round numbers somebody liked: each one has
// a measured requirement behind it, listed in the `needs` column, and the test
// exists so a future change to the template that makes the document taller
// fails here instead of silently producing a second page.
func TestPageHeightCoversTheMeasuredContent(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		outLegs    int
		inLegs     int
		passengers int
		needsMM    float64
		wantMM     int
	}{
		{name: "one leg, one passenger", outLegs: 1, passengers: 1, needsMM: 137.2, wantMM: 160},
		{name: "two legs, one passenger", outLegs: 2, passengers: 1, needsMM: 162.7, wantMM: 180},
		{name: "three legs, two passengers", outLegs: 3, passengers: 2, needsMM: 221.7, wantMM: 280},
		{name: "round trip, one passenger", outLegs: 1, inLegs: 1, passengers: 1, needsMM: 184.5, wantMM: 200},
		{name: "round trip of three, one passenger", outLegs: 3, inLegs: 3, passengers: 1, needsMM: 303.0, wantMM: 340},
		{name: "one leg, four passengers", outLegs: 1, passengers: 4, needsMM: 237.6, wantMM: 260},
		{name: "round trip of three, four passengers", outLegs: 3, inLegs: 3, passengers: 4, needsMM: 465.5, wantMM: 500},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := pageHeightMM(shapedModel(testCase.outLegs, testCase.inLegs, testCase.passengers))

			if got != testCase.wantMM {
				t.Errorf("pageHeightMM = %dmm, want %dmm", got, testCase.wantMM)
			}
			// The assertion that actually matters. Everything else is bookkeeping;
			// this is the property - the paper has to be at least as tall as the
			// document, or the PDF comes out in two pieces.
			if float64(got) < testCase.needsMM {
				t.Errorf("pageHeightMM = %dmm but this shape renders %.1fmm of content - it will break onto a second page",
					got, testCase.needsMM)
			}
		})
	}
}

func TestPageHeightIsClamped(t *testing.T) {
	// A ticket with nothing on it still has to be a page somebody can print.
	if got := pageHeightMM(ViewModel{}); got != pageMinMM {
		t.Errorf("an empty ticket asks for %dmm, want the %dmm floor", got, pageMinMM)
	}

	// Past the ceiling, breaking is the honest answer: no paper size makes a
	// forty-leg itinerary readable on one sheet.
	if got := pageHeightMM(shapedModel(20, 20, 20)); got != pageMaxMM {
		t.Errorf("an enormous ticket asks for %dmm, want the %dmm ceiling", got, pageMaxMM)
	}
}

// shapedModel builds a view model with the right *shape* - the height only ever
// depends on how many journeys, segments and passengers there are, never on
// what is written in them.
func shapedModel(outLegs, inLegs, passengers int) ViewModel {
	var model ViewModel

	if outLegs > 0 {
		model.Journeys = append(model.Journeys, Journey{Segments: make([]Segment, outLegs)})
	}
	if inLegs > 0 {
		model.Journeys = append(model.Journeys, Journey{Segments: make([]Segment, inLegs), Inbound: true})
	}
	model.Passengers = make([]Passenger, passengers)

	return model
}
