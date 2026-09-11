package main

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/utils"
)

// A fake CPF that fails the check digits is the quietest possible bug here: nothing errors,
// the dump loads, and every document in the panel is simply rejected as invalid the first
// time somebody edits a person. So the generator is checked against the very validator the
// application uses.
func TestFakeCPFIsAcceptedByNormalizeCPF(t *testing.T) {
	seeds := []string{
		"00000000-0000-0000-0000-000000000000",
		"a", "b", "c",
		"3f2504e0-4f89-11d3-9a0c-0305e82c3301",
		"ffffffffffffffffffffffffffffffff",
	}
	for _, seed := range seeds {
		cpf := fakeCPF(seed)
		if len(cpf) != 11 {
			t.Fatalf("fakeCPF(%q) = %q, want 11 digits", seed, cpf)
		}
		if _, ok := utils.NormalizeCPF(cpf); !ok {
			t.Errorf("fakeCPF(%q) = %q, rejected by utils.NormalizeCPF", seed, cpf)
		}
	}
}

// Identity is the property the whole design rests on: two rows that held the same CPF must
// still hold the same one, and two that differed must still differ.
func TestFakeCPFIsDeterministicAndDistinct(t *testing.T) {
	first, second := fakeCPF("same"), fakeCPF("same")
	if first != second {
		t.Errorf("fakeCPF is not deterministic: %q then %q", first, second)
	}
	if fakeCPF("one") == fakeCPF("two") {
		t.Error("different seeds collided")
	}
}

// The repeated-digit placeholders (00000000000, 11111111111) pass the check digits but are
// rejected by NormalizeCPF on purpose, so the generator has to steer around them.
func TestFakeCPFAvoidsRepeatedDigits(t *testing.T) {
	for _, seed := range []string{"x", "y", "z", "0", "1"} {
		cpf := fakeCPF(seed)
		allSame := true
		for i := 1; i < len(cpf); i++ {
			if cpf[i] != cpf[0] {
				allSame = false
				break
			}
		}
		if allSame {
			t.Errorf("fakeCPF(%q) = %q, all digits equal", seed, cpf)
		}
	}
}

func TestFakePhoneIsUniquePerCounter(t *testing.T) {
	seen := map[string]bool{}
	for i := 1; i <= 1000; i++ {
		phone := fakePhone(i)
		if seen[phone] {
			t.Fatalf("fakePhone(%d) = %q collided", i, phone)
		}
		seen[phone] = true
	}
}

// The panel and the bot parse locators, so a scrambled one has to keep the shape the real one
// had - otherwise the environment exercises a code path production never takes.
func TestScrambleCodeKeepsShape(t *testing.T) {
	cases := []struct{ code, keepPrefix, shape string }{
		{"ABC123", "", "AAA999"},
		{"*QWERTY*", "", "*AAAAAA*"},
		{"XYZ87A - mala", "", "AAA99A - mala"},
		{"LA1234567XRNH", "LA", "LA9999999AAAA"},
	}
	for _, c := range cases {
		got, err := scrambleCode(c.code, c.keepPrefix)
		if err != nil {
			t.Fatal(err)
		}
		shape := []byte(got)
		for i, ch := range shape {
			switch {
			case i < len(c.keepPrefix):
			case ch >= 'A' && ch <= 'Z':
				shape[i] = 'A'
			case ch >= '0' && ch <= '9':
				shape[i] = '9'
			}
		}
		if string(shape) != c.shape {
			t.Errorf("scrambleCode(%q) = %q, shape %q, want %q", c.code, got, shape, c.shape)
		}
	}
}
