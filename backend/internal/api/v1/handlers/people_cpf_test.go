package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The registry is the largest pile of CPFs the system holds, and until migration 38 it was
// readable by anyone with a dump. What matters here is that the number stops being stored,
// that it keeps behaving like a number for everyone using the panel, and that the rows written
// before the change keep working while the backfill has not reached them.

// rawCPF reads the column without going through AfterFind, which is the only way to see what
// is actually on disk.
func rawCPF(t *testing.T, tx *gorm.DB, personID string) string {
	t.Helper()
	var stored string
	err := tx.Table("people").Select("cpf").Where("id = ?", personID).Scan(&stored).Error
	if err != nil {
		t.Fatalf("failed to read the stored CPF: %v", err)
	}
	return stored
}

func TestPersonCPFIsEncryptedAtRestAndReadableBack(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		person := data.Person{Name: "Alice Ferreira", CPF: strPtr(cpfAlice)}
		create(t, tx, &person)

		stored := rawCPF(t, tx, person.ID)
		if !strings.HasPrefix(stored, "enc:") {
			t.Fatalf("stored CPF = %q, want an enc: value", stored)
		}
		if strings.Contains(stored, "52998224725") || strings.Contains(stored, cpfAlice) {
			t.Fatalf("the number survived in the column: %q", stored)
		}

		// Staff still need to read it to log into the program on the client's behalf, so it
		// comes back in the clear - exactly like Account.Password.
		var found data.Person
		if err := tx.First(&found, "id = ?", person.ID).Error; err != nil {
			t.Fatalf("failed to read the person back: %v", err)
		}
		if found.CPF == nil || *found.CPF != cpfAlice {
			t.Errorf("read back %v, want %q", found.CPF, cpfAlice)
		}
		if found.CPFFingerprint == nil {
			t.Error("the person got no fingerprint, so nothing enforces uniqueness")
		}
	})
}

func TestTheSameCPFCannotBeRegisteredTwice(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		first := data.Person{Name: "Alice Ferreira", CPF: strPtr(cpfAlice)}
		create(t, tx, &first)

		// Ciphertext is different every time, so only the fingerprint can catch this. Typed
		// with a mask the second time, to prove the normalization happens before the index.
		second := data.Person{Name: "Alice F.", CPF: strPtr("52998224725")}
		if err := tx.Create(&second).Error; err == nil {
			t.Fatal("the same CPF was accepted on two people")
		}
	})
}

func TestAPersonAndABeneficiaryShareTheFingerprint(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		person := data.Person{Name: "Alice Ferreira", CPF: strPtr(cpfAlice)}
		create(t, tx, &person)

		// This is what lets an account holder be recognised now that neither side stores the
		// number: same key, same normalization, same digest.
		normalized, ok := utils.NormalizeCPF(cpfAlice)
		if !ok {
			t.Fatal("the fixture CPF should be valid")
		}
		expected, err := utils.Fingerprint(normalized)
		if err != nil {
			t.Fatalf("Fingerprint returned an error: %v", err)
		}
		if person.CPFFingerprint == nil || *person.CPFFingerprint != expected {
			t.Errorf("person fingerprint = %v, want the same digest a beneficiary would get", person.CPFFingerprint)
		}
	})
}

func TestAnInvalidCPFStillGetsUniqueness(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		// The column has always accepted whatever was typed, and the old unique index covered
		// it. Fingerprinting only valid documents would have quietly dropped that.
		first := data.Person{Name: "Typo One", CPF: strPtr("111.222.333-44")}
		create(t, tx, &first)
		if first.CPFFingerprint == nil {
			t.Fatal("an invalid CPF got no fingerprint, so it is no longer unique")
		}

		second := data.Person{Name: "Typo Two", CPF: strPtr("11122233344")}
		if err := tx.Create(&second).Error; err == nil {
			t.Error("the same invalid CPF was accepted on two people")
		}
	})
}

func TestClearingTheCPFReleasesIt(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		person := data.Person{Name: "Alice Ferreira", CPF: strPtr(cpfAlice)}
		create(t, tx, &person)

		person.CPF = nil
		if err := tx.Save(&person).Error; err != nil {
			t.Fatalf("failed to clear the CPF: %v", err)
		}
		if person.CPFFingerprint != nil {
			t.Error("the fingerprint outlived the CPF, so the document stays reserved")
		}

		// Somebody else can now be registered with it, which is the point.
		other := data.Person{Name: "Alice Ferreira Again", CPF: strPtr(cpfAlice)}
		if err := tx.Create(&other).Error; err != nil {
			t.Errorf("the document stayed reserved after being cleared: %v", err)
		}
	})
}

func TestLegacyPlaintextRowsKeepWorkingUntilTheBackfillRuns(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		// A row exactly as it looked before migration 38: plain text, no fingerprint. Written
		// with raw SQL because the hooks would encrypt anything that went through the model.
		person := data.Person{Name: "Legacy Person"}
		create(t, tx, &person)
		err := tx.Exec("UPDATE people SET cpf = ?, cpf_fingerprint = NULL WHERE id = ?",
			"52998224725", person.ID).Error
		if err != nil {
			t.Fatalf("failed to plant the legacy row: %v", err)
		}

		// Decrypt passes unprefixed values through, so the panel still shows the number.
		var found data.Person
		if err := tx.First(&found, "id = ?", person.ID).Error; err != nil {
			t.Fatalf("failed to read the legacy person: %v", err)
		}
		if found.CPF == nil || *found.CPF != "52998224725" {
			t.Fatalf("legacy CPF read back as %v, want the plain number", found.CPF)
		}

		if err := data.BackfillPersonCPFs(tx); err != nil {
			t.Fatalf("BackfillPersonCPFs returned an error: %v", err)
		}

		if stored := rawCPF(t, tx, person.ID); !strings.HasPrefix(stored, "enc:") {
			t.Errorf("after the backfill the column still holds %q", stored)
		}
		var backfilled data.Person
		if err := tx.First(&backfilled, "id = ?", person.ID).Error; err != nil {
			t.Fatalf("failed to read the person after the backfill: %v", err)
		}
		if backfilled.CPF == nil || *backfilled.CPF != "52998224725" {
			t.Errorf("after the backfill the CPF reads as %v", backfilled.CPF)
		}
		if backfilled.CPFFingerprint == nil {
			t.Error("the backfill left the row without a fingerprint")
		}

		// Running it again must not double-encrypt: the enc: prefix is the guard.
		if err := data.BackfillPersonCPFs(tx); err != nil {
			t.Fatalf("the second run returned an error: %v", err)
		}
		var twice data.Person
		if err := tx.First(&twice, "id = ?", person.ID).Error; err != nil {
			t.Fatalf("failed to read the person after the second run: %v", err)
		}
		if twice.CPF == nil || *twice.CPF != "52998224725" {
			t.Errorf("the second run mangled the CPF into %v", twice.CPF)
		}
	})
}

func TestSearchingPeopleByCPFNeedsTheWholeDocument(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		person := data.Person{Name: "Zoraide Buscavel", CPF: strPtr(cpfAlice)}
		create(t, tx, &person)

		found := func(term string) bool {
			rec := httptest.NewRecorder()
			NewPeopleHandler(tx).GetPeople(rec,
				httptest.NewRequest(http.MethodGet, "/people?q="+term, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			return strings.Contains(rec.Body.String(), "Zoraide Buscavel")
		}

		// Whole document, masked or bare - both normalize to the same fingerprint.
		if !found("52998224725") {
			t.Error("the bare CPF did not find the person")
		}
		if !found("529.982.247-25") {
			t.Error("the masked CPF did not find the person")
		}

		// A fragment used to work and deliberately does not any more: there is nothing to
		// match a piece of a document against once the column is ciphertext.
		if found("52998") {
			t.Error("a partial CPF still matched, so something is still searching plain text")
		}

		// The rest of the search is untouched.
		if !found("Zoraide") {
			t.Error("searching by name stopped working")
		}
	})
}

func TestSearchingFindsAPersonWhoseCPFIsInvalid(t *testing.T) {

	// The registry has always taken whatever was typed, so some documents fail the check
	// digits. They are still the only way anybody finds those people, so the search has to
	// normalize the way the index does rather than validating first.
	runInTx(t, func(tx *gorm.DB) {
		person := data.Person{Name: "Wanderleia Digitada", CPF: strPtr("12345678901")}
		create(t, tx, &person)

		rec := httptest.NewRecorder()
		NewPeopleHandler(tx).GetPeople(rec,
			httptest.NewRequest(http.MethodGet, "/people?q=12345678901", nil))
		if !strings.Contains(rec.Body.String(), "Wanderleia Digitada") {
			t.Error("a person registered with an invalid CPF cannot be found by that CPF")
		}
	})
}
