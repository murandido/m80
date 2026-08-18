package data

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// BackfillPersonCPFs encrypts the CPFs that predate migration 38 and gives each one its
// fingerprint. It runs at boot, right after the migrations, because SQL cannot do it: the key
// is ENCRYPTION_KEY and it only exists in this process.
//
// Idempotent and cheap to leave in place. Once every row is encrypted the query below matches
// nothing, so a normal boot pays for one indexless scan of a table with a few thousand rows.
//
// Not a seed, and deliberately not in seed_*.go: this rewrites operational data rather than
// reconciling a catalog. It touches exactly the rows that are still plaintext, one at a time,
// and says how many it changed.
func BackfillPersonCPFs(db *gorm.DB) error {
	var people []Person
	// Unscoped: a soft-deleted person still holds a readable CPF, which is the whole point of
	// this. Person has no soft delete today, but the Unscoped costs nothing and stops this
	// from quietly skipping rows if that ever changes.
	//
	// The condition reads the raw column, so it has to run before AfterFind decrypts anything -
	// which is exactly what a WHERE does.
	err := db.Unscoped().
		Where("cpf IS NOT NULL AND cpf NOT LIKE 'enc:%'").
		Find(&people).Error
	if err != nil {
		return fmt.Errorf("BackfillPersonCPFs: reading the people still in plain text: %w", err)
	}

	if len(people) == 0 {
		return nil
	}

	slog.Info("encrypting the CPFs left in plain text", "people", len(people))

	for _, person := range people {
		// The hook is called by hand rather than left to GORM: it derives the fingerprint from
		// the plaintext and then encrypts it, and doing it here means the two columns below are
		// written from values this code can see. Calling it twice would be harmless anyway - the
		// "enc:" prefix makes it a no-op.
		if err := person.BeforeSave(db); err != nil {
			slog.Error("could not prepare a person's CPF for encryption",
				"person_id", person.ID, "err", err)
			continue
		}

		// Exactly two columns, so nothing else about the person is rewritten - a backfill that
		// also touched, say, complete would be a data migration nobody asked for.
		err := db.Model(&Person{}).
			Where("id = ?", person.ID).
			Updates(map[string]any{
				"cpf":             person.CPF,
				"cpf_fingerprint": person.CPFFingerprint,
			}).Error
		if err != nil {
			// Named row by row: the likely failure is a duplicate fingerprint, meaning two
			// people already share a CPF the old index somehow let through. That needs a human,
			// and stopping the boot over it would take the whole API down with it.
			slog.Error("could not encrypt a person's CPF",
				"person_id", person.ID, "err", err)
			continue
		}
	}

	slog.Info("finished encrypting the CPFs left in plain text")
	return nil
}
