package injunction

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/testdb"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testdb.RequireTestDatabase(t)
	testDBOnce.Do(func() {
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"), os.Getenv("DB_PORT"))
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			testDBErr = err
			return
		}
		if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
			testDBErr = fmt.Errorf("no database")
			return
		}
		data.Connect()
		testDB = data.DB
	})
	if testDBErr != nil {
		t.Skipf("skipping injunction database tests: %v", testDBErr)
	}
	return testDB
}

func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)
	t.Parallel()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	fn(tx)
}

// Each test package draws phones from its own million; see legacyimport's nextPhone.
var phoneCounter int64 = 6_000_000

func mustCreate(t *testing.T, tx *gorm.DB, value any) {
	t.Helper()
	if err := tx.Create(value).Error; err != nil {
		t.Fatalf("seeding %T: %v", value, err)
	}
}

type world struct {
	airline  data.Airline
	program  data.LoyaltyProgram
	agency   data.Agency
	issuer   data.User
	hudson   data.Account
	outsider data.Account
	group    data.InjunctionGroup
	label    string
}

// seedWorld creates a program, an injunction account the Tropa maps ("HUDSON"), an account
// the workbook also carries but the Tropa does not ("SILVIA"), and the issuer "PEDRO".
func seedWorld(t *testing.T, tx *gorm.DB) world {
	t.Helper()
	suffix := uuid.New().String()[:8]
	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreate(t, tx, &airline)
	// Not AZUL or SMILES: the test programs have no statement, which leaves the workbook as
	// the only source and every "missing" row unconfirmed.
	program := data.LoyaltyProgram{Name: "TROPA " + suffix, Code: "TR" + suffix, DefaultAirlineID: airline.ID}
	mustCreate(t, tx, &program)
	agency := data.Agency{Name: "Agência " + suffix}
	mustCreate(t, tx, &agency)
	issuer := data.User{Name: "Pedro " + suffix, Phone: fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)), Password: "secret123"}
	mustCreate(t, tx, &issuer)

	account := func(name string, category string) data.Account {
		holder := data.Person{Name: name + " " + suffix}
		mustCreate(t, tx, &holder)
		acc := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID, Category: category}
		mustCreate(t, tx, &acc)
		return acc
	}
	hudson := account("Hudson", data.AccountCategoryInjunction)
	outsider := account("Silvia", "standard")
	mustCreate(t, tx, &data.AccountNickname{AccountID: outsider.ID, LoyaltyProgramID: program.ID, Nickname: "SILVIA" + suffix})

	group := data.InjunctionGroup{
		Name:     "Tropa " + suffix,
		AgencyID: agency.ID,
		Mappings: data.InjunctionGroupMappings{
			Accounts: []data.InjunctionAccountMapping{{ProgramCode: program.Code, AccountLabel: "HUDSON", AccountID: hudson.ID}},
			Issuers:  []data.InjunctionIssuerMapping{{IssuerName: "PEDRO", UserID: issuer.ID}},
		},
	}
	mustCreate(t, tx, &group)
	return world{airline: airline, program: program, agency: agency, issuer: issuer, hudson: hudson, outsider: outsider, group: group, label: "SILVIA" + suffix}
}

// workbook builds a Tropa-layout day sheet: header on row 2, rows from row 3.
func workbook(t *testing.T, sheetName string, rows [][]any) []byte {
	t.Helper()
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	if _, err := file.NewSheet(sheetName); err != nil {
		t.Fatal(err)
	}
	set := func(axis string, value any) {
		if err := file.SetCellValue(sheetName, axis, value); err != nil {
			t.Fatal(err)
		}
	}
	set("A1", "CONTROLE TROPA AZUL 1 - 20/11/2026")
	header := []string{"Nº", "Companhia", "Localizador", "Data do Voo", "Passageiro Principal", "Origem", "Destino",
		"Pontos", "Milheiro", "Total", "Conta", "Emissor", "Observação", "Taxa de Resgate", "Extrato Conferido"}
	for i, label := range header {
		column, _ := excelize.ColumnNumberToName(i + 1)
		set(column+"2", label)
	}
	for r, row := range rows {
		for c, value := range row {
			if value == nil {
				continue
			}
			column, _ := excelize.ColumnNumberToName(c + 1)
			set(fmt.Sprintf("%s%d", column, r+3), value)
		}
	}
	if index, err := file.GetSheetIndex("Sheet1"); err == nil && index >= 0 {
		_ = file.DeleteSheet("Sheet1")
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// row is one workbook line: program, locator, points, milheiro (reais), account, issuer,
// redemption fee (reais).
func row(n int, program, locator string, points int, rate float64, account, issuer string, fee float64) []any {
	return []any{n, program, locator, nil, "PASSAGEIRO TESTE", "REC", "GRU", points, rate,
		float64(points) / 1000 * rate, account, issuer, "-", fee, false}
}

var day = time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)

func TestCheckSortsTheTropaRowsIntoBuckets(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx)
		code := w.program.Code

		// Already in the panel with a different rate: must come back as a difference.
		existing := data.Issuance{
			LoyaltyProgramID: w.program.ID, AirlineID: w.airline.ID, ProgramLocator: "PANEL1", Points: 10000, ValueOfTheThousand: 1500,
			AccountID: w.hudson.ID, AgencyID: w.agency.ID, IssuerID: w.issuer.ID, Active: true,
		}
		if err := tx.Omit("Account", "Agency", "Issuer", "LoyaltyProgram", "Airline").Create(&existing).Error; err != nil {
			t.Fatalf("seeding issuance: %v", err)
		}

		content := workbook(t, "20-11-26", [][]any{
			row(1, code, "NEW001", 20000, 16, "HUDSON ", "PEDRO", 69.9), // missing in panel; fee must be ignored
			row(2, code, "PANEL1", 10000, 16, "HUDSON", "PEDRO", 0),     // differs: milheiro 1500 x 1600
			row(3, code, "OTHER1", 5000, 15, w.label, "PEDRO", 0),       // another account: ignored
			row(4, code, "NOMAP1", 7000, 15, "HUDSON", "FULANO", 0),     // issuer without mapping
			{5, code, "EMPTY1"}, // locator without data
		})

		result, err := Check(tx, w.group, content, day, day)
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		locators := func(items []Item) string {
			var out []string
			for _, item := range items {
				out = append(out, item.Locator)
			}
			return strings.Join(out, ",")
		}
		if got := locators(result.MissingInPanelUnconfirmed); got != "NEW001" {
			t.Errorf("unconfirmed: got %q", got)
		}
		if got := locators(result.DiffersFromPanel); got != "PANEL1" {
			t.Errorf("differs: got %q (%+v)", got, result.DiffersFromPanel)
		}
		if got := locators(result.NeedsMapping); got != "NOMAP1" {
			t.Errorf("needs mapping: got %q", got)
		}
		if got := locators(result.Invalid); got != "EMPTY1" {
			t.Errorf("invalid: got %q", got)
		}
		for _, bucket := range [][]Item{result.MissingInPanelUnconfirmed, result.DiffersFromPanel, result.NeedsMapping, result.Invalid} {
			if strings.Contains(locators(bucket), "OTHER1") {
				t.Errorf("a row of an account outside the Tropa was read")
			}
		}
	})
}

func TestImportWritesOnlyTheTropaRowsWithoutTheFee(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx)
		code := w.program.Code
		content := workbook(t, "20-11-26", [][]any{
			row(1, code, "NEW001", 20000, 16, "HUDSON", "PEDRO", 69.9),
			row(2, code, "OTHER1", 5000, 15, w.label, "PEDRO", 0),
		})

		// Asking for the outsider's row by number must not import it.
		result, err := Import(tx, w.group, content, day, nil, w.issuer.ID)
		if err != nil {
			t.Fatalf("import: %v", err)
		}
		if result.Imported != 1 {
			t.Fatalf("imported %d, want 1 (%+v)", result.Imported, result)
		}

		var imported data.Issuance
		if err := tx.First(&imported, "program_locator = ?", "NEW001").Error; err != nil {
			t.Fatalf("NEW001 was not written: %v", err)
		}
		if imported.AccountID != w.hudson.ID || imported.Points != 20000 || imported.ValueOfTheThousand != 1600 {
			t.Errorf("imported issuance: %+v", imported)
		}
		if imported.Taxes != nil && *imported.Taxes != 0 {
			t.Errorf("the redemption fee became taxes: %d", *imported.Taxes)
		}
		var outsiders int64
		tx.Model(&data.Issuance{}).Where("program_locator = ?", "OTHER1").Count(&outsiders)
		if outsiders != 0 {
			t.Error("a row of an account outside the Tropa was imported")
		}
	})
}

// azulProgram returns the AZUL program, creating it inside the transaction when the test
// database has none: the reconciliation reads a statement only for AZUL and SMILES accounts.
func azulProgram(t *testing.T, tx *gorm.DB) (data.LoyaltyProgram, data.Airline) {
	t.Helper()
	var program data.LoyaltyProgram
	if err := tx.Where("code = ?", "AZUL").First(&program).Error; err == nil {
		var airline data.Airline
		if err := tx.First(&airline, "id = ?", program.DefaultAirlineID).Error; err != nil {
			t.Fatal(err)
		}
		return program, airline
	}
	suffix := uuid.New().String()[:8]
	airline := data.Airline{Name: "Azul Test " + suffix, Code: "AZ" + suffix}
	mustCreate(t, tx, &airline)
	program = data.LoyaltyProgram{Name: "Azul", Code: "AZUL", DefaultAirlineID: airline.ID}
	mustCreate(t, tx, &program)
	return program, airline
}

// The bucket the agent may import without asking: the workbook row and the airline statement
// agree on locator and points, and the panel does not have the booking. A statement that
// disagrees on the points must not count as confirmation, and a statement booking the
// workbook never mentions is reported on its own.
func TestCheckConfirmsAWorkbookRowTheStatementBacks(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		program, _ := azulProgram(t, tx)
		suffix := uuid.New().String()[:8]
		agency := data.Agency{Name: "Agência " + suffix}
		mustCreate(t, tx, &agency)
		issuer := data.User{Name: "Pedro " + suffix, Phone: fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)), Password: "secret123"}
		mustCreate(t, tx, &issuer)
		holder := data.Person{Name: "Hudson " + suffix}
		mustCreate(t, tx, &holder)
		account := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID, Category: data.AccountCategoryInjunction}
		mustCreate(t, tx, &account)
		group := data.InjunctionGroup{
			Name: "Tropa " + suffix, AgencyID: agency.ID,
			Mappings: data.InjunctionGroupMappings{
				Accounts: []data.InjunctionAccountMapping{{ProgramCode: "AZUL", AccountLabel: "HUDSON", AccountID: account.ID}},
				Issuers:  []data.InjunctionIssuerMapping{{IssuerName: "PEDRO", UserID: issuer.ID}},
			},
		}
		mustCreate(t, tx, &group)

		statementLine := func(locator string, points int) {
			loc := locator
			mustCreate(t, tx, &data.AzulStatementLine{
				AccountID: account.ID, OccurredOn: day, Kind: data.AzulLineRedemption, Points: -points, Locator: &loc,
			})
		}
		confirmed, mismatch, onlyStatement := "CF"+strings.ToUpper(suffix[:4]), "MM"+strings.ToUpper(suffix[:4]), "ST"+strings.ToUpper(suffix[:4])
		statementLine(confirmed, 30000)
		statementLine(mismatch, 45000)
		statementLine(onlyStatement, 12000)

		content := workbook(t, "20-11-26", [][]any{
			row(1, "AZUL", confirmed, 30000, 16, "HUDSON", "PEDRO", 0),
			row(2, "AZUL", mismatch, 40000, 16, "HUDSON", "PEDRO", 0),
		})
		result, err := Check(tx, group, content, day, day)
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		if len(result.Statements) != 1 || result.Statements[0].Error != "" {
			t.Fatalf("the statement was not read: %+v", result.Statements)
		}
		if len(result.MissingInPanelConfirmed) != 1 || result.MissingInPanelConfirmed[0].Locator != confirmed {
			t.Errorf("confirmed: got %+v", result.MissingInPanelConfirmed)
		}
		if len(result.MissingInPanelUnconfirmed) != 1 || result.MissingInPanelUnconfirmed[0].Locator != mismatch ||
			result.MissingInPanelUnconfirmed[0].StatementPoints == nil || *result.MissingInPanelUnconfirmed[0].StatementPoints != 45000 {
			t.Errorf("points that disagree must stay unconfirmed: got %+v", result.MissingInPanelUnconfirmed)
		}
		if len(result.MissingInSheetAndPanel) != 1 || result.MissingInSheetAndPanel[0].Locator != onlyStatement {
			t.Errorf("statement only: got %+v", result.MissingInSheetAndPanel)
		}
	})
}
