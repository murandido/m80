package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/legacyimport"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// buildLegacyWorkbook renders a one-day, one-block workbook: the shape of the operators'
// spreadsheet, cut down to the two rows a test needs.
func buildLegacyWorkbook(t *testing.T, programLabel, accountLabel string, locators ...string) []byte {
	t.Helper()

	file := excelize.NewFile()
	defer func() { _ = file.Close() }()

	const sheet = "01-08-26"
	if _, err := file.NewSheet(sheet); err != nil {
		t.Fatalf("creating the sheet: %v", err)
	}

	set := func(axis string, value any) {
		if err := file.SetCellValue(sheet, axis, value); err != nil {
			t.Fatalf("setting %s: %v", axis, err)
		}
	}

	set("A1", "CONTROLE DE EMISSÕES - 01/08/2026 - MARCUS")
	// The whole header row, because the columns are read off it by name - the workbooks
	// in use lay them out four different ways.
	for column, label := range map[string]string{
		"A2": "Nº", "B2": "Companhia", "C2": "Localizador", "D2": "Data do Voo",
		"E2": "Passageiro Principal", "F2": "Origem", "G2": "Destino", "H2": "Pontos",
		"I2": "Milheiro", "J2": "Taxa", "K2": "Total", "L2": "Conta", "M2": "Observação",
	} {
		set(column, label)
	}

	for index, locator := range locators {
		row := 3 + index
		set(fmt.Sprintf("A%d", row), index+1)
		set(fmt.Sprintf("B%d", row), programLabel)
		set(fmt.Sprintf("C%d", row), locator)
		set(fmt.Sprintf("E%d", row), "RENAN RIBEIRO")
		set(fmt.Sprintf("H%d", row), 18)
		set(fmt.Sprintf("I%d", row), 16.25)
		set(fmt.Sprintf("L%d", row), accountLabel)
	}
	set(fmt.Sprintf("A%d", 3+len(locators)), "TOTAL FATURAMENTO")

	if index, err := file.GetSheetIndex("Sheet1"); err == nil && index >= 0 {
		if err := file.DeleteSheet("Sheet1"); err != nil {
			t.Fatalf("deleting the default sheet: %v", err)
		}
	}

	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("writing the workbook: %v", err)
	}
	return buffer.Bytes()
}

// postLegacyImport builds the multipart request both routes take and runs it through the
// handler as an authenticated admin.
func postLegacyImport(
	t *testing.T,
	tx *gorm.DB,
	handle func(*LegacyImportHandler, http.ResponseWriter, *http.Request),
	userID string,
	content []byte,
	fields map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	return postLegacyImportAs(t, tx, handle, userID, content, fields, access.FullScope())
}

// postLegacyImportAs is postLegacyImport for a caller restricted to the given agencies.
func postLegacyImportAs(
	t *testing.T,
	tx *gorm.DB,
	handle func(*LegacyImportHandler, http.ResponseWriter, *http.Request),
	userID string,
	content []byte,
	fields map[string]string,
	scope access.AgencyScope,
) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	part, err := form.CreateFormFile("file", "planilha.xlsx")
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}
	for name, value := range fields {
		if err := form.WriteField(name, value); err != nil {
			t.Fatalf("writing the %q field: %v", name, err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/issuances/legacy-import", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req = withScope(req.WithContext(asUser(req.Context(), data.User{ID: userID, Role: data.RoleAdmin})), scope)

	rec := httptest.NewRecorder()
	handle(&LegacyImportHandler{DB: tx, Broker: events.NewBroker()}, rec, req)
	return rec
}

// legacyWorld is the cast one imported issuance needs, plus the labels the spreadsheet
// will name them by.
type legacyWorld struct {
	program data.LoyaltyProgram
	agency  data.Agency
	user    data.User
}

func seedLegacyWorld(t *testing.T, tx *gorm.DB) legacyWorld {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	if err := tx.Create(&airline).Error; err != nil {
		t.Fatalf("seeding the airline: %v", err)
	}
	program := data.LoyaltyProgram{
		Name: "Azul " + suffix, Code: "AZUL" + suffix, DefaultAirlineID: airline.ID,
	}
	if err := tx.Create(&program).Error; err != nil {
		t.Fatalf("seeding the loyalty program: %v", err)
	}
	agency := data.Agency{Name: "Agência " + suffix}
	if err := tx.Create(&agency).Error; err != nil {
		t.Fatalf("seeding the agency: %v", err)
	}
	user := data.User{Name: "Marcus", Phone: fmt.Sprintf("1199%07d", atomic.AddInt64(&phoneCounter, 1)%10000000), Password: "secret123"}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatalf("seeding the user: %v", err)
	}

	holder := data.Person{Name: "Marise Andrade " + suffix}
	if err := tx.Create(&holder).Error; err != nil {
		t.Fatalf("seeding the holder: %v", err)
	}
	account := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}
	if err := tx.Create(&account).Error; err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	alias := data.AccountNickname{
		AccountID: account.ID, LoyaltyProgramID: program.ID, Nickname: "MARISE " + suffix,
	}
	if err := tx.Create(&alias).Error; err != nil {
		t.Fatalf("seeding the alias: %v", err)
	}

	return legacyWorld{program: program, agency: agency, user: user}
}

func decodeLegacyPreview(t *testing.T, rec *httptest.ResponseRecorder) schemas.LegacyImportPreviewResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data schemas.LegacyImportPreviewResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding the preview: %v (body %s)", err, rec.Body.String())
	}
	return envelope.Data
}

func TestPreviewLegacyImport_ListsTheDaysWithoutWritingAnything(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedLegacyWorld(t, tx)
		content := buildLegacyWorkbook(t, w.program.Code, "MARISE", "EQF8VK", "LNS6MD")

		rec := postLegacyImport(t, tx,
			(*LegacyImportHandler).PreviewLegacyImport, w.user.ID, content, nil)

		preview := decodeLegacyPreview(t, rec)
		if len(preview.Days) != 1 {
			t.Fatalf("days = %+v, want one", preview.Days)
		}
		if preview.Days[0].Date != "2026-08-01" || preview.Days[0].RowCount != 2 {
			t.Errorf("day = %+v, want 2026-08-01 with 2 rows", preview.Days[0])
		}
		// No day was asked for, so there is nothing to detail.
		if preview.Preview != nil {
			t.Errorf("preview = %+v, want nil until a day is chosen", preview.Preview)
		}

		var count int64
		tx.Model(&data.Issuance{}).Where("program_locator IN ?", []string{"EQF8VK", "LNS6MD"}).Count(&count)
		if count != 0 {
			t.Errorf("the preview wrote %d issuances, want 0", count)
		}
	})
}

func TestPreviewLegacyImport_DetailsTheChosenDay(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedLegacyWorld(t, tx)
		content := buildLegacyWorkbook(t, w.program.Code, "MARISE", "EQF8VK")

		mappings, err := json.Marshal(legacyimport.Mappings{
			AgencyID: w.agency.ID,
			Issuers:  []legacyimport.IssuerMapping{{IssuerName: "MARCUS", UserID: w.user.ID}},
		})
		if err != nil {
			t.Fatalf("encoding the mappings: %v", err)
		}

		rec := postLegacyImport(t, tx,
			(*LegacyImportHandler).PreviewLegacyImport, w.user.ID, content, map[string]string{
				"day":      "2026-08-01",
				"mappings": string(mappings),
			})

		preview := decodeLegacyPreview(t, rec)
		if preview.Preview == nil {
			t.Fatal("preview = nil, want the day's rows")
		}
		if preview.Preview.AgencyID != w.agency.ID {
			t.Errorf("agency = %q, want the chosen %q", preview.Preview.AgencyID, w.agency.ID)
		}
		// The alias is unknown to the database, so the screen is told to ask - which
		// is what the mapping controls exist for.
		if len(preview.Preview.PendingAccounts) != 1 {
			t.Errorf("pending accounts = %+v, want one", preview.Preview.PendingAccounts)
		}
	})
}

func TestPreviewLegacyImport_RefusesAFileThatIsNotTheSpreadsheet(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedLegacyWorld(t, tx)

		rec := postLegacyImport(t, tx,
			(*LegacyImportHandler).PreviewLegacyImport, w.user.ID, []byte("not a workbook"), nil)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestCommitLegacyImport_WritesOnlyTheSelectedRows(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedLegacyWorld(t, tx)
		content := buildLegacyWorkbook(t, w.program.Code, "MARISE", "EQF8VK", "LNS6MD")

		// One account mapping standing for the operator picking the account, and the
		// issuer named because "MARCUS" alone could be several people.
		var account data.Account
		if err := tx.Where("loyalty_program_id = ?", w.program.ID).First(&account).Error; err != nil {
			t.Fatalf("loading the seeded account: %v", err)
		}
		mappings, err := json.Marshal(legacyimport.Mappings{
			AgencyID: w.agency.ID,
			Issuers:  []legacyimport.IssuerMapping{{IssuerName: "MARCUS", UserID: w.user.ID}},
			Accounts: []legacyimport.AccountMapping{{
				ProgramCode: w.program.Code, AccountLabel: "MARISE", AccountID: account.ID,
			}},
		})
		if err != nil {
			t.Fatalf("encoding the mappings: %v", err)
		}

		// Only the first row is ticked - the case of a block the operator already
		// registered through the panel.
		rows, err := json.Marshal([]legacyimport.RowSelection{{BlockIndex: 0, Number: 3}})
		if err != nil {
			t.Fatalf("encoding the selection: %v", err)
		}

		rec := postLegacyImport(t, tx,
			(*LegacyImportHandler).CommitLegacyImport, w.user.ID, content, map[string]string{
				"day":      "2026-08-01",
				"mappings": string(mappings),
				"rows":     string(rows),
			})

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "EQF8VK").First(&issuance).Error; err != nil {
			t.Fatalf("the selected row was not written: %v", err)
		}
		if !issuance.Imported {
			t.Error("imported = false, want true")
		}

		var unselected int64
		tx.Model(&data.Issuance{}).Where("program_locator = ?", "LNS6MD").Count(&unselected)
		if unselected != 0 {
			t.Errorf("the unticked row was written anyway (%d rows)", unselected)
		}
	})
}

func TestCommitLegacyImport_RefusesAnEmptySelection(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedLegacyWorld(t, tx)
		content := buildLegacyWorkbook(t, w.program.Code, "MARISE", "EQF8VK")

		rec := postLegacyImport(t, tx,
			(*LegacyImportHandler).CommitLegacyImport, w.user.ID, content, map[string]string{
				"day":  "2026-08-01",
				"rows": "[]",
			})

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestCommitLegacyImport_RefusesADayTheFileDoesNotHave(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedLegacyWorld(t, tx)
		content := buildLegacyWorkbook(t, w.program.Code, "MARISE", "EQF8VK")

		rec := postLegacyImport(t, tx,
			(*LegacyImportHandler).CommitLegacyImport, w.user.ID, content, map[string]string{
				"day":  "2026-08-02",
				"rows": `[{"block_index":0,"number":3}]`,
			})

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
		}
	})
}
