package promotion

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

func strPtr(v string) *string { return &v }

// A nil selector is not the same as an empty one to us, but it has to become one
// on the wire: the service reads "" as "resolve this heuristically", and sending
// a stored recipe's blanks any other way is how a campaign gets submitted against
// a form nobody chose.
func TestWireRecipe(t *testing.T) {
	t.Parallel()

	t.Run("no recipe stays no recipe", func(t *testing.T) {
		t.Parallel()
		if got := wireRecipe(nil); got != nil {
			t.Fatalf("wireRecipe(nil) = %+v, want nil", got)
		}
	})

	t.Run("nil selectors become the empty string", func(t *testing.T) {
		t.Parallel()
		got := wireRecipe(&data.PromoRecipe{
			CPFSelector: strPtr("#cpf"),
			WaitSeconds: 4,
		})

		if got.CPFSelector != "#cpf" {
			t.Fatalf("CPFSelector = %q, want %q", got.CPFSelector, "#cpf")
		}
		for name, value := range map[string]string{
			"CheckboxSelector": got.CheckboxSelector,
			"SubmitSelector":   got.SubmitSelector,
			"SuccessPattern":   got.SuccessPattern,
			"FailurePattern":   got.FailurePattern,
		} {
			if value != "" {
				t.Fatalf("%s = %q, want the empty string the service reads as 'resolve it'", name, value)
			}
		}
		if got.WaitSeconds != 4 {
			t.Fatalf("WaitSeconds = %d, want 4", got.WaitSeconds)
		}
	})
}

// The number the group is told. Only outright successes count: an unknown verdict
// is exactly the case where the service could not tell, and reporting it as a
// success is how a CPF that never got enrolled stops being retried.
func TestRunResultSucceeded(t *testing.T) {
	t.Parallel()

	result := RunResult{Outcomes: []Outcome{
		{Status: data.PromoEnrollmentSuccess},
		{Status: data.PromoEnrollmentFailure},
		{Status: data.PromoEnrollmentSuccess},
		{Status: data.PromoEnrollmentUnknown},
	}}

	if got := result.Succeeded(); got != 2 {
		t.Fatalf("Succeeded() = %d, want 2 - only the outright successes", got)
	}
	if got := (RunResult{}).Succeeded(); got != 0 {
		t.Fatalf("Succeeded() on an empty run = %d, want 0", got)
	}
}

// A screenshot is a nicety attached to a real enrolment, so an undecodable one
// returns nil rather than an error: failing the enrolment over its picture would
// throw away the part that mattered.
func TestDecodeScreenshot(t *testing.T) {
	t.Parallel()

	if got := DecodeScreenshot(""); got != nil {
		t.Fatalf("DecodeScreenshot(\"\") = %v, want nil", got)
	}
	if got := DecodeScreenshot("!!not base64!!"); got != nil {
		t.Fatalf("DecodeScreenshot(garbage) = %v, want nil", got)
	}
	// "hi" in standard base64.
	if got := DecodeScreenshot("aGk="); string(got) != "hi" {
		t.Fatalf("DecodeScreenshot() = %q, want %q", got, "hi")
	}
}
