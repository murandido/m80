// Command anonymize rewrites, in place, every piece of personal data in a database
// restored from a production dump, so the result can be loaded into a non-production
// environment - today, the CA's Mode box the designer works against.
//
// It is deliberately a separate command rather than a migration or a seed: it must never
// run as part of booting anything. The only safe way to reach it is the pipeline in
// scripts/db-anonymize.sh, which restores a dump into a throwaway database first.
//
// Two properties are worth stating up front, because both look like the obvious
// alternative and neither is:
//
//   - It never decrypts anything. Every encrypted column is overwritten with a freshly
//     encrypted fake value, so the production ENCRYPTION_KEY is not needed here and must
//     not be supplied. The keys in the environment are the *destination* environment's.
//   - It preserves identity, not values. Two rows that held the same CPF still hold the
//     same (fake) CPF afterwards, because the fake is keyed off the old blind index. That
//     is what keeps a person, their loyalty documents and their beneficiary rows pointing
//     at each other once the real numbers are gone.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
	"github.com/murandido/m-80-backend/internal/utils"
)

// Every anonymised database is created by scripts/db-anonymize.sh under this prefix, and
// this command refuses to touch anything else. There is no flag to override it: the whole
// point is that pointing this at the live database has to be impossible, not discouraged.
const requiredDBPrefix = "m80_anon_"

// The one password every user and every loyalty account ends up with. It is printed at the
// end so whoever loaded the dump can actually log in.
const testPassword = "ca-mode-1234"

func main() {
	log.SetFlags(0)

	if !strings.HasPrefix(os.Getenv("DB_NAME"), requiredDBPrefix) {
		// The name itself is not echoed back: it arrives from the environment and this
		// message goes to a log. Whoever set it can read it more easily than this can
		// safely print it.
		log.Fatalf("refusing to run: DB_NAME must start with %q.\n"+
			"This command rewrites every personal field in the database it is pointed at.\n"+
			"Use scripts/db-anonymize.sh, which restores the dump into a throwaway database first.",
			requiredDBPrefix)
	}

	// Fail before touching a row if the destination keys are missing, rather than halfway
	// through with a table already rewritten.
	if _, err := utils.Encrypt("probe"); err != nil {
		log.Fatalf("ENCRYPTION_KEY: %v", err)
	}
	if _, err := utils.Fingerprint("probe"); err != nil {
		log.Fatalf("CPF_INDEX_KEY: %v", err)
	}

	db, err := sql.Open("postgres", fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_NAME"),
	))
	if err != nil {
		log.Fatalf("connecting: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		log.Fatalf("connecting: %v", err)
	}

	// One transaction for the whole run. A half-anonymised database is worse than none:
	// it looks scrubbed and is not.
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	run := &anonymiser{tx: tx, cpfByOldFingerprint: map[string]string{}}
	if err := run.all(); err != nil {
		log.Fatalf("❌ %v", err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("commit: %v", err)
	}

	log.Println()
	log.Printf("✅ anonymised. Every user and loyalty account password is now %q.", testPassword)
	run.printLogins()
}

type anonymiser struct {
	tx *sql.Tx
	// Old blind index -> the fake CPF that replaced it. The old fingerprints were computed
	// under production's CPF_INDEX_KEY, which we do not have and do not need: equal
	// fingerprints meant equal CPFs, and that is the only fact being preserved.
	cpfByOldFingerprint map[string]string
	logins              []string
}

func (a *anonymiser) all() error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"people", a.people},
		{"loyalty_documents", a.loyaltyDocuments},
		{"account_beneficiaries", a.accountBeneficiaries},
		{"accounts", a.accounts},
		{"issuance_passengers", a.issuancePassengers},
		{"issuances", a.issuances},
		{"point_batches", a.pointBatches},
		{"account_nicknames", a.accountNicknames},
		{"agencies", a.agencies},
		{"sale_payments", a.salePayments},
		{"sales", a.sales},
		{"airline statements", a.airlineStatements},
		{"clients", a.clients},
		{"users", a.users},
		{"person_nicknames", a.personNicknames},
		{"promo_enrollments", a.promoEnrollments},
		{"free-text and log tables", a.truncateTables},
		{"whatsmeow session", a.dropWhatsmeow},
	}
	for _, step := range steps {
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

// collect runs query and gathers every row through scan. Reading a table fully before
// updating it, rather than writing through a live cursor, keeps each step to one obvious
// shape and avoids updating a result set that is still open.
func collect[T any](tx *sql.Tx, query string, scan func(*sql.Rows) (T, error)) ([]T, error) {
	rows, err := tx.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []T
	for rows.Next() {
		value, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// keyed is a row reduced to what anonymising it needs: its primary key, and one nullable
// column whose presence decides how the row is treated - a blind index on the tables that
// carry one, the encrypted document on issuance_passengers.
type keyed struct {
	id  string
	ref sql.NullString
}

func scanKeyed(rows *sql.Rows) (keyed, error) {
	var row keyed
	err := rows.Scan(&row.id, &row.ref)
	return row, err
}

func scanID(rows *sql.Rows) (string, error) {
	var id string
	err := rows.Scan(&id)
	return id, err
}

// cpfFor returns the fake CPF standing in for a row that used to carry oldFingerprint,
// minting one the first time that fingerprint is seen. A row with no fingerprint at all
// gets a fresh CPF keyed off its own id, since there is nothing to tie it to.
func (a *anonymiser) cpfFor(oldFingerprint sql.NullString, id string) string {
	if !oldFingerprint.Valid || oldFingerprint.String == "" {
		return fakeCPF(id)
	}
	if cpf, ok := a.cpfByOldFingerprint[oldFingerprint.String]; ok {
		return cpf
	}
	cpf := fakeCPF(oldFingerprint.String)
	a.cpfByOldFingerprint[oldFingerprint.String] = cpf
	return cpf
}

// nameSeed picks what a row's fake name is derived from. Rows that shared a blind index were
// the same human being, so they have to end up sharing a name too - a person and their own
// beneficiary row carrying the same CPF under two different names is exactly the kind of
// thing that sends someone hunting for a bug in the panel.
func nameSeed(oldFingerprint sql.NullString, id string) string {
	if oldFingerprint.Valid && oldFingerprint.String != "" {
		return oldFingerprint.String
	}
	return id
}

func (a *anonymiser) people() error {
	people, err := collect(a.tx, `SELECT id, cpf_fingerprint FROM people ORDER BY id`, scanKeyed)
	if err != nil {
		return err
	}

	for i, person := range people {
		// A person whose CPF was never filled in keeps it empty: the panel treats a missing
		// document as an incomplete registration, and inventing one here would quietly mark
		// every such row complete.
		var encrypted, fingerprint any
		if person.ref.Valid && person.ref.String != "" {
			cpf := a.cpfFor(person.ref, person.id)
			enc, err := utils.Encrypt(cpf)
			if err != nil {
				return err
			}
			fp, err := utils.Fingerprint(cpf)
			if err != nil {
				return err
			}
			encrypted, fingerprint = enc, fp
		}

		first, last := fakeName(nameSeed(person.ref, person.id))
		_, err := a.tx.Exec(`
			UPDATE people
			   SET name = $2, cpf = $3, cpf_fingerprint = $4,
			       email = $5, phone = $6,
			       zip_code = NULL, state = NULL, city = NULL,
			       address = NULL, address_number = NULL,
			       complement = NULL, neighborhood = NULL
			 WHERE id = $1`,
			person.id, first+" "+last, encrypted, fingerprint,
			fmt.Sprintf("pessoa%d@exemplo.test", i+1), fakePhone(i+1),
		)
		if err != nil {
			return err
		}
	}
	log.Printf("  people: %d rows", len(people))
	return nil
}

func (a *anonymiser) loyaltyDocuments() error {
	docs, err := collect(a.tx,
		`SELECT id, cpf_fingerprint FROM loyalty_documents ORDER BY id`, scanKeyed)
	if err != nil {
		return err
	}

	for _, doc := range docs {
		cpf := a.cpfFor(doc.ref, doc.id)
		encrypted, err := utils.Encrypt(cpf)
		if err != nil {
			return err
		}
		fingerprint, err := utils.Fingerprint(cpf)
		if err != nil {
			return err
		}
		first, last := fakeName(nameSeed(doc.ref, doc.id))
		if _, err := a.tx.Exec(
			`UPDATE loyalty_documents SET name = $2, cpf = $3, cpf_fingerprint = $4 WHERE id = $1`,
			doc.id, first+" "+last, encrypted, fingerprint,
		); err != nil {
			return err
		}
	}
	log.Printf("  loyalty_documents: %d rows", len(docs))
	return nil
}

func (a *anonymiser) accountBeneficiaries() error {
	type beneficiaryRow struct {
		keyed
		accountID, oldNormalizedName string
	}
	beneficiaries, err := collect(a.tx,
		`SELECT id, cpf_fingerprint, account_id, normalized_name FROM account_beneficiaries ORDER BY id`,
		func(rows *sql.Rows) (beneficiaryRow, error) {
			var row beneficiaryRow
			err := rows.Scan(&row.id, &row.ref, &row.accountID, &row.oldNormalizedName)
			return row, err
		})
	if err != nil {
		return err
	}

	// A beneficiary without a CPF is identified by its name - (account_id, normalized_name)
	// is unique among them - so its fake name is seeded by the old name, the same way a CPF
	// row is seeded by its old fingerprint. Distinct real names can still land on the same
	// fake one, and inside one account that would break the index, so a taken name is
	// re-rolled with a counter until it is free.
	//
	// The index is checked row by row, not at commit, so a fake name could also collide
	// with a real name on a row not yet rewritten. Parking every name-only row on its own
	// id first means the only names left to collide with are fake ones already tracked.
	if _, err := a.tx.Exec(`
		UPDATE account_beneficiaries SET normalized_name = 'pending:' || id
		 WHERE cpf_fingerprint IS NULL`); err != nil {
		return err
	}
	nameOnlyTaken := map[string]bool{}
	for _, beneficiary := range beneficiaries {
		hasCPF := beneficiary.ref.Valid && beneficiary.ref.String != ""
		seed := nameSeed(beneficiary.ref, beneficiary.id)
		if !hasCPF {
			seed = "name:" + beneficiary.oldNormalizedName
		}
		first, last := fakeName(seed)
		name := first + " " + last
		for attempt := 1; !hasCPF && nameOnlyTaken[beneficiary.accountID+"|"+utils.NormalizeName(name)]; attempt++ {
			first, last = fakeName(seed + "#" + strconv.Itoa(attempt))
			name = first + " " + last
		}
		if !hasCPF {
			nameOnlyTaken[beneficiary.accountID+"|"+utils.NormalizeName(name)] = true
		}

		// A beneficiary known by name only keeps being known by name only - the null
		// fingerprint is what tells the matcher this person never arrived with a document.
		var fingerprint, masked any
		if hasCPF {
			cpf := a.cpfFor(beneficiary.ref, beneficiary.id)
			fp, err := utils.Fingerprint(cpf)
			if err != nil {
				return err
			}
			fingerprint, masked = fp, utils.MaskCPF(cpf)
		}

		if _, err := a.tx.Exec(`
			UPDATE account_beneficiaries
			   SET name = $2, normalized_name = $3, cpf_fingerprint = $4, cpf_masked = $5
			 WHERE id = $1`,
			beneficiary.id, name, utils.NormalizeName(name), fingerprint, masked,
		); err != nil {
			return err
		}
	}
	log.Printf("  account_beneficiaries: %d rows", len(beneficiaries))
	return nil
}

func (a *anonymiser) accounts() error {
	encrypted, err := utils.Encrypt(testPassword)
	if err != nil {
		return err
	}
	// Every account gets the same ciphertext. That is fine here and would not be in the
	// application: GCM's random nonce exists so equal plaintexts are not visibly equal, and
	// in a database where every password is knowingly identical there is nothing to hide.
	result, err := a.tx.Exec(`
		UPDATE accounts
		   SET login = 'conta-' || left(id::text, 8) || '@exemplo.test',
		       password = $1
		 WHERE login IS NOT NULL OR password IS NOT NULL`, encrypted)
	if err != nil {
		return err
	}
	if err := logAffected("accounts", "rows", result); err != nil {
		return err
	}
	// The Azul customer key and the Smiles refresh token are live sessions with the airline,
	// not something a fake password makes harmless: carried over, the sync loops would read
	// real statements from the CA's Mode box. Clearing them is also what keeps those loops
	// off every account (they only pick up rows where it is set), while tracks_statement
	// stays as is so the statement screens still show the imported lines.
	result, err = a.tx.Exec(`
		UPDATE accounts SET azul_customer_key = NULL, smiles_refresh_token = NULL
		 WHERE azul_customer_key IS NOT NULL OR smiles_refresh_token IS NOT NULL`)
	if err != nil {
		return err
	}
	return logAffected("accounts", "airline sessions cleared", result)
}

func (a *anonymiser) issuancePassengers() error {
	passengers, err := collect(a.tx,
		`SELECT id, document FROM issuance_passengers ORDER BY id`, scanKeyed)
	if err != nil {
		return err
	}

	for _, passenger := range passengers {
		first, last := fakeName(passenger.id)

		// Only a passenger who arrived with a document gets one back: a null here is the
		// difference between "we know this person's CPF" and "we never learned it", and the
		// beneficiary matcher reads exactly that distinction.
		var document any
		if passenger.ref.Valid && passenger.ref.String != "" {
			enc, err := utils.Encrypt(fakeCPF(passenger.id))
			if err != nil {
				return err
			}
			document = enc
		}
		if _, err := a.tx.Exec(
			`UPDATE issuance_passengers SET first_name = $2, last_name = $3, document = $4 WHERE id = $1`,
			passenger.id, strings.ToUpper(first), strings.ToUpper(last), document,
		); err != nil {
			return err
		}
	}
	log.Printf("  issuance_passengers: %d rows", len(passengers))
	return nil
}

func (a *anonymiser) issuances() error {
	// main_passenger_name is denormalised from the passenger list, so it is rebuilt from the
	// names just written rather than invented again - otherwise the header of an issuance
	// would name somebody who is not on it.
	if _, err := a.tx.Exec(`
		UPDATE issuances i
		   SET main_passenger_name = p.first_name || ' ' || p.last_name
		  FROM issuance_passengers p
		 WHERE p.issuance_id = i.id
		   AND p.position = (SELECT min(position) FROM issuance_passengers WHERE issuance_id = i.id)
		   AND i.main_passenger_name IS NOT NULL`); err != nil {
		return err
	}

	// Older issuances carry the header name with no passenger list behind it, so there is
	// nothing to rebuild from and the statement above leaves them untouched.
	orphans, err := collect(a.tx, `
		SELECT id FROM issuances i
		 WHERE main_passenger_name IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM issuance_passengers p WHERE p.issuance_id = i.id)
		 ORDER BY id`, scanID)
	if err != nil {
		return err
	}
	for _, id := range orphans {
		first, last := fakeName(id)
		if _, err := a.tx.Exec(`UPDATE issuances SET main_passenger_name = $2 WHERE id = $1`,
			id, strings.ToUpper(first+" "+last)); err != nil {
			return err
		}
	}
	log.Printf("  issuances: %d header names without a passenger list", len(orphans))

	if err := a.bookingReferences(); err != nil {
		return err
	}

	// Observations are free text an operator typed, so they can hold anything - a phone
	// number, a passenger's real name, a payment detail. Whether an issuance *has* one is
	// visible in the grid, so the column is replaced rather than emptied.
	result, err := a.tx.Exec(`
		UPDATE issuances
		   SET observation = 'Observação de exemplo (conteúdo original removido).'
		 WHERE observation IS NOT NULL AND observation <> ''`)
	if err != nil {
		return err
	}
	return logAffected("issuances", "observations replaced", result)
}

// bookingReferences replaces the codes that open a real booking on an airline's site. A
// locator plus a surname is all "manage my booking" asks for, and the surname alone is not
// enough protection to leave the code as it is.
//
// The shape is kept - letters stay letters, digits stay digits, the " - mala" an operator
// appended stays - because the panel and the bot parse these, and a code that no longer
// looks like one would exercise a different path than production does. The same old code
// always maps to the same new one, so issuances that shared a booking still share it.
func (a *anonymiser) bookingReferences() error {
	columns := []struct {
		name, selectQuery, updateQuery, keepPrefix string
	}{
		{"program_locator",
			`SELECT id, program_locator FROM issuances WHERE program_locator <> '' ORDER BY id`,
			`UPDATE issuances SET program_locator = $2 WHERE id = $1`, ""},
		{"airline_locator",
			`SELECT id, airline_locator FROM issuances WHERE airline_locator <> '' ORDER BY id`,
			`UPDATE issuances SET airline_locator = $2 WHERE id = $1`, ""},
		// LATAM purchase numbers start with a fixed "LA" that identifies the kind of code,
		// not the booking.
		{"latam_purchase_number",
			`SELECT id, latam_purchase_number FROM issuances WHERE latam_purchase_number <> '' ORDER BY id`,
			`UPDATE issuances SET latam_purchase_number = $2 WHERE id = $1`, "LA"},
	}

	for _, column := range columns {
		rows, err := collect(a.tx, column.selectQuery, scanKeyed)
		if err != nil {
			return err
		}

		// Every old code is reserved up front. program_locator is unique among active rows
		// and the index is checked per row, so a new code equal to an old one not yet
		// rewritten would fail the run - and a new code equal to an old one would not be
		// anonymous anyway.
		taken := map[string]bool{}
		for _, row := range rows {
			taken[row.ref.String] = true
		}
		replacement := map[string]string{}
		for _, row := range rows {
			old := row.ref.String
			code, seen := replacement[old]
			for !seen {
				code, err = scrambleCode(old, column.keepPrefix)
				if err != nil {
					return err
				}
				// A value with no letter or digit to swap - an operator typed a word into the
				// field - comes back unchanged and is reserved, so without the second test this
				// loop never ends. Such a value names no booking and is kept as it is.
				seen = !taken[code] || code == old
			}
			replacement[old], taken[code] = code, true

			if _, err := a.tx.Exec(column.updateQuery, row.id, code); err != nil {
				return err
			}
		}
		log.Printf("  issuances: %d %s replaced", len(rows), column.name)
	}
	return nil
}

// accountNicknames rewrites the aliases the bot and the grid use for a loyalty account,
// which are usually the holder's real name - "MARIA (JOÃO P.)" and the like.
func (a *anonymiser) accountNicknames() error {
	type nicknameRow struct{ id, programID string }
	nicknames, err := collect(a.tx, `SELECT id, loyalty_program_id FROM account_nicknames ORDER BY id`,
		func(rows *sql.Rows) (nicknameRow, error) {
			var row nicknameRow
			err := rows.Scan(&row.id, &row.programID)
			return row, err
		})
	if err != nil {
		return err
	}

	// Same two moves as the name-only beneficiaries: park every row first so a fake alias
	// cannot hit a real one still waiting its turn, then re-roll within a programme until free.
	if _, err := a.tx.Exec(`UPDATE account_nicknames SET nickname = 'pending:' || id`); err != nil {
		return err
	}
	taken := map[string]bool{}
	for _, nickname := range nicknames {
		seed := nickname.id
		first, last := fakeName(seed)
		alias := strings.ToUpper(first + " " + last)
		for attempt := 1; taken[nickname.programID+"|"+alias]; attempt++ {
			first, last = fakeName(seed + "#" + strconv.Itoa(attempt))
			alias = strings.ToUpper(first + " " + last)
		}
		taken[nickname.programID+"|"+alias] = true

		if _, err := a.tx.Exec(`UPDATE account_nicknames SET nickname = $2 WHERE id = $1`,
			nickname.id, alias); err != nil {
			return err
		}
	}
	log.Printf("  account_nicknames: %d rows", len(nicknames))
	return nil
}

// Agencies are partners, but in a business this size a partner is often a person trading
// under their own name, and the alias the bot matches on is that name shortened.
func (a *anonymiser) agencies() error {
	if _, err := a.tx.Exec(`
		UPDATE agencies AS g SET name = 'Agência ' || numbered.position
		  FROM (SELECT id, row_number() OVER (ORDER BY created_at, id) AS position FROM agencies) AS numbered
		 WHERE g.id = numbered.id`); err != nil {
		return err
	}
	result, err := a.tx.Exec(`
		UPDATE agency_nicknames AS n SET nickname = 'agencia' || numbered.position
		  FROM (SELECT id, row_number() OVER (ORDER BY id) AS position FROM agency_nicknames) AS numbered
		 WHERE n.id = numbered.id`)
	if err != nil {
		return err
	}
	return logAffected("agencies", "nicknames replaced", result)
}

// salePayments replaces the operator's note on a payment, free text like every other note.
func (a *anonymiser) salePayments() error {
	result, err := a.tx.Exec(`
		UPDATE sale_payments SET note = 'Nota de exemplo (conteúdo original removido).'
		 WHERE note IS NOT NULL AND note <> ''`)
	if err != nil {
		return err
	}
	return logAffected("sale_payments", "notes replaced", result)
}

// sales replaces the note on the payments screen - "paga na sexta", "cobrar do Fulano".
func (a *anonymiser) sales() error {
	result, err := a.tx.Exec(`
		UPDATE sales SET note = 'Nota de exemplo (conteúdo original removido).'
		 WHERE note IS NOT NULL AND note <> ''`)
	if err != nil {
		return err
	}
	return logAffected("sales", "notes replaced", result)
}

// airlineStatements scrubs the lines read from Azul and Smiles. The airline's description
// of a transfer or a booking can carry the name of whoever sent the points or flew, so it
// falls back to the type's own wording, which is what tells the lines apart on screen. The
// locator goes because it opens the booking on the airline's site, and the issuances'
// locators were already scrambled without a map to follow here. The operator's comment is
// free text like every other note. The sync state's detail is the last error the airline
// returned, which may quote the account.
func (a *anonymiser) airlineStatements() error {
	queries := []struct{ table, sql string }{
		{"azul_statement_lines", `
			UPDATE azul_statement_lines
			   SET description = type_description, locator = NULL,
			       comment = CASE WHEN comment IS NULL OR comment = '' THEN comment
			                      ELSE 'Comentário de exemplo (conteúdo original removido).' END`},
		{"smiles_statement_lines", `
			UPDATE smiles_statement_lines SET description = type_description, locator = NULL`},
		{"azul_sync_state", `UPDATE azul_sync_state SET detail = NULL WHERE detail IS NOT NULL`},
		{"smiles_sync_state", `UPDATE smiles_sync_state SET detail = NULL WHERE detail IS NOT NULL`},
	}
	for _, query := range queries {
		result, err := a.tx.Exec(query.sql)
		if err != nil {
			return fmt.Errorf("%s: %w", query.table, err)
		}
		if err := logAffected(query.table, "rows scrubbed", result); err != nil {
			return err
		}
	}
	return nil
}

// A batch's note is either what the operator typed or the airline's own wording copied from
// the statement, and a transfer's wording can name the person who sent the points. It is
// numbered rather than made uniform because the note is what tells two batches of the same
// origin apart in the grid. statement_lines is left alone: it holds only days and amounts.
func (a *anonymiser) pointBatches() error {
	result, err := a.tx.Exec(`
		UPDATE point_batches AS b
		   SET note = 'Lote de exemplo ' || numbered.position
		  FROM (SELECT id, row_number() OVER (ORDER BY acquired_on, id) AS position
		          FROM point_batches
		         WHERE note IS NOT NULL AND note <> '') AS numbered
		 WHERE b.id = numbered.id`)
	if err != nil {
		return err
	}
	return logAffected("point_batches", "notes replaced", result)
}

func (a *anonymiser) clients() error {
	ids, err := collect(a.tx, `SELECT id FROM clients ORDER BY id`, scanID)
	if err != nil {
		return err
	}

	// clients.name carries a unique index, so the counter carries the uniqueness and the
	// fake name only has to look plausible.
	for i, id := range ids {
		first, last := fakeName(id)
		if _, err := a.tx.Exec(
			`UPDATE clients SET name = $2, phone = $3 WHERE id = $1`,
			id, fmt.Sprintf("%s %s %d", first, last, i+1), fakePhone(i+1),
		); err != nil {
			return err
		}
	}
	log.Printf("  clients: %d rows", len(ids))
	return nil
}

// user is the one row shape that needs a second column for its own sake rather than to
// decide a branch: the role is printed with the new phone, so whoever loads the dump knows
// which login gets them into which part of the panel.
type user struct {
	id, role string
}

func (a *anonymiser) users() error {
	hashed, err := utils.HashPassword(testPassword)
	if err != nil {
		return err
	}

	users, err := collect(a.tx, `SELECT id, role FROM users ORDER BY created_at, id`,
		func(rows *sql.Rows) (user, error) {
			var row user
			err := rows.Scan(&row.id, &row.role)
			return row, err
		})
	if err != nil {
		return err
	}

	for i, u := range users {
		first, last := fakeName(u.id)
		name := first + " " + last
		phone := fakePhone(i + 1)
		if _, err := a.tx.Exec(
			`UPDATE users SET name = $2, phone = $3, password = $4 WHERE id = $1`,
			u.id, name, phone, hashed,
		); err != nil {
			return err
		}
		a.logins = append(a.logins, fmt.Sprintf("    %-10s %s  (%s)", u.role, phone, name))
	}
	log.Printf("  users: %d rows", len(users))
	return nil
}

func (a *anonymiser) personNicknames() error {
	// The alias of a person is usually just their first name, so it is regenerated from the
	// same seed - and the counter keeps the unique index happy.
	ids, err := collect(a.tx, `SELECT id FROM person_nicknames ORDER BY id`, scanID)
	if err != nil {
		return err
	}

	for i, id := range ids {
		first, _ := fakeName(id)
		if _, err := a.tx.Exec(
			`UPDATE person_nicknames SET nickname = $2 WHERE id = $1`,
			id, fmt.Sprintf("%s%d", strings.ToLower(first), i+1),
		); err != nil {
			return err
		}
	}
	log.Printf("  person_nicknames: %d rows", len(ids))
	return nil
}

func (a *anonymiser) promoEnrollments() error {
	// chat_jid is a real WhatsApp address. The enrolment history is worth keeping; who asked
	// for it, from which number, is not.
	result, err := a.tx.Exec(`UPDATE promo_enrollments SET chat_jid = NULL WHERE chat_jid IS NOT NULL`)
	if err != nil {
		return err
	}
	if err := logAffected("promo_enrollments", "chat addresses cleared", result); err != nil {
		return err
	}

	// message is the report the bot sent back to the group, and it quotes every CPF it
	// enrolled in plain text.
	result, err = a.tx.Exec(`
		UPDATE promo_enrollments SET message = 'Relatório de exemplo (conteúdo original removido).'
		 WHERE message IS NOT NULL AND message <> ''`)
	if err != nil {
		return err
	}
	return logAffected("promo_enrollments", "reports replaced", result)
}

func (a *anonymiser) truncateTables() error {
	// audit_logs stores before/after payloads of every write, so it holds a copy of every
	// value the tables above just had scrubbed - anonymising it row by row would mean
	// re-implementing all of the above against free-form JSON.
	//
	// listened_chats is the bot's routing table, keyed by real WhatsApp addresses. It is
	// also the wrong thing to carry over: the CA's Mode bot is never paired, so a chat list
	// there would describe conversations that environment cannot have.
	//
	// booking_lookup_attempts is a temporary report of production's calls to the airlines,
	// and its error column quotes whatever the airline answered about a real booking.
	for _, table := range []string{"audit_logs", "listened_chats", "booking_lookup_attempts"} {
		// The name is a literal from the slice above, not input.
		if _, err := a.tx.Exec(fmt.Sprintf(`TRUNCATE TABLE %s CASCADE`, table)); err != nil {
			return err
		}
		log.Printf("  %s: truncated", table)
	}
	return nil
}

func (a *anonymiser) dropWhatsmeow() error {
	// The whatsmeow session lives in this same database (cmd/bot/main.go), so a dump loaded
	// somewhere else wakes up paired to the real business WhatsApp: a second bot reading
	// customers' chats and able to answer them. Dropping the tables is what makes that
	// impossible rather than merely unlikely - whatsmeow recreates them empty on boot and
	// GetFirstDevice then finds nothing, leaving the bot waiting to be paired.
	tables, err := collect(a.tx, `
		SELECT quote_ident(tablename) FROM pg_tables
		 WHERE schemaname = 'public' AND tablename LIKE 'whatsmeow%'
		 ORDER BY tablename`, scanID)
	if err != nil {
		return err
	}

	if len(tables) == 0 {
		// Not an error, but loud: a dump with no session tables is either already scrubbed
		// or not the dump anybody thought it was.
		log.Println("  whatsmeow: no session tables found (already scrubbed?)")
		return nil
	}

	// The identifiers come back from pg_tables already quoted by Postgres itself.
	if _, err := a.tx.Exec(
		fmt.Sprintf(`DROP TABLE %s CASCADE`, strings.Join(tables, ", ")),
	); err != nil {
		return err
	}
	log.Printf("  whatsmeow: %d session tables dropped", len(tables))
	return nil
}

func logAffected(table, noun string, result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	log.Printf("  %s: %d %s", table, affected, noun)
	return nil
}

func (a *anonymiser) printLogins() {
	if len(a.logins) == 0 {
		log.Println("No users in the dump - the API will seed ADMIN_PHONE/ADMIN_PASSWORD on first boot.")
		return
	}
	log.Println()
	log.Println("Log in with any of:")
	for _, line := range a.logins {
		log.Println(line)
	}
}

// --- deterministic fake values -------------------------------------------------------
//
// Everything below is keyed off a seed string (a row id, or an old blind index) so that a
// second run over the same dump produces the same database, and so that rows that shared a
// value still share one afterwards.

func seedOf(seed, salt string) uint64 {
	sum := sha256.Sum256([]byte(salt + ":" + seed))
	return binary.BigEndian.Uint64(sum[:8])
}

var firstNames = []string{
	"Ana", "Bruno", "Carla", "Diego", "Elisa", "Felipe", "Gabriela", "Henrique",
	"Isabela", "João", "Karina", "Lucas", "Mariana", "Nelson", "Olivia", "Paulo",
	"Queila", "Rafael", "Sofia", "Thiago", "Ursula", "Vinícius", "Wagner", "Yasmin",
}

var lastNames = []string{
	"Almeida", "Barbosa", "Cardoso", "Duarte", "Esteves", "Ferreira", "Gomes",
	"Henriques", "Iglesias", "Jardim", "Klein", "Lima", "Moreira", "Nogueira",
	"Oliveira", "Pereira", "Quintana", "Ribeiro", "Santos", "Teixeira",
}

func fakeName(seed string) (first, last string) {
	return firstNames[seedOf(seed, "first")%uint64(len(firstNames))],
		lastNames[seedOf(seed, "last")%uint64(len(lastNames))]
}

// fakePhone builds a Brazilian mobile number from a counter, so the result is unique by
// construction - users.phone carries a unique index and a hash-derived number would only be
// unique most of the time.
func fakePhone(n int) string {
	return fmt.Sprintf("119%08d", n)
}

// scrambleCode swaps every uppercase letter for a random uppercase letter and every digit for
// a random digit, leaving keepPrefix and anything else in place. It draws from crypto/rand
// rather than from a seed on purpose: a six-character locator has about two billion values,
// so a code derived from a hash of the real one could be walked back by brute force.
func scrambleCode(code, keepPrefix string) (string, error) {
	const letters, digits = "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "0123456789"
	pick := func(alphabet string) (byte, error) {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return 0, err
		}
		return alphabet[n.Int64()], nil
	}

	prefix := ""
	if keepPrefix != "" && strings.HasPrefix(code, keepPrefix) {
		prefix, code = keepPrefix, code[len(keepPrefix):]
	}
	out := []byte(code)
	for i, c := range out {
		var err error
		switch {
		case c >= 'A' && c <= 'Z':
			out[i], err = pick(letters)
		case c >= '0' && c <= '9':
			out[i], err = pick(digits)
		}
		if err != nil {
			return "", err
		}
	}
	return prefix + string(out), nil
}

// fakeCPF returns 11 digits that pass the Receita Federal check-digit rule, because
// utils.NormalizeCPF rejects anything that does not and the panel would then treat every
// document in the database as invalid.
func fakeCPF(seed string) string {
	digits := make([]int, 11)
	value := seedOf(seed, "cpf")
	for i := range 9 {
		digits[i] = int(value % 10)
		value /= 10
	}

	// Repeated-digit documents are rejected by NormalizeCPF as placeholders even though they
	// pass the check digits, so nudge the number off that case.
	allSame := true
	for _, digit := range digits[1:9] {
		if digit != digits[0] {
			allSame = false
			break
		}
	}
	if allSame {
		digits[8] = (digits[8] + 1) % 10
	}

	digits[9] = checkDigit(digits[:9])
	digits[10] = checkDigit(digits[:10])

	var out strings.Builder
	for _, digit := range digits {
		out.WriteString(strconv.Itoa(digit))
	}
	return out.String()
}

func checkDigit(digits []int) int {
	sum := 0
	weight := len(digits) + 1
	for _, digit := range digits {
		sum += digit * weight
		weight--
	}
	remainder := (sum * 10) % 11
	if remainder == 10 {
		return 0
	}
	return remainder
}
