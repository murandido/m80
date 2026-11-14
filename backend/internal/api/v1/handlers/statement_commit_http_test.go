package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// statementProgram is what differs between the Azul and the Smiles commit: where the synced
// credit lives and which handler reads it. Everything the commit decides after that - the
// agency check, the grouping, the skip - is shared, and so are these tests.
type statementProgram struct {
	name       string
	seedCredit func(t *testing.T, tx *gorm.DB, accountID string, points int)
	commit     func(handler *StatementHandler) http.HandlerFunc
}

var statementPrograms = []statementProgram{
	{
		name: "azul",
		seedCredit: func(t *testing.T, tx *gorm.DB, accountID string, points int) {
			t.Helper()
			description := "Resgate pontos Inter"
			line := data.AzulStatementLine{
				AccountID: accountID, OccurredOn: day(t, "2026-09-04"), Kind: data.AzulLineAccrual,
				Points: points, Description: &description,
			}
			create(t, tx, &line)
		},
		commit: func(handler *StatementHandler) http.HandlerFunc { return handler.CommitAzulStatement },
	},
	{
		name: "smiles",
		seedCredit: func(t *testing.T, tx *gorm.DB, accountID string, points int) {
			t.Helper()
			description := "Transferência Livelo"
			line := data.SmilesStatementLine{
				AccountID: accountID, OccurredOn: day(t, "2026-09-04"), Kind: data.SmilesLineAccrual,
				Points: points, Description: &description,
			}
			create(t, tx, &line)
		},
		commit: func(handler *StatementHandler) http.HandlerFunc { return handler.CommitSmilesStatement },
	},
}

func callStatementCommit(tx *gorm.DB, program statementProgram, accountID string, scope access.AgencyScope, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/statement/commit", strings.NewReader(body))
	req = withScope(withIDs(req, map[string]string{"id": accountID}), scope)
	rec := httptest.NewRecorder()
	program.commit(NewStatementHandler(tx))(rec, req)
	return rec
}

func statementSelection(index int, originID, fundingAgencyID string, costCents int) string {
	return fmt.Sprintf(`{"index":%d,"origin_id":"%s","cost_cents":%d,"funding_agency_id":"%s"}`,
		index, originID, costCents, fundingAgencyID)
}

func statementCommitBody(selections ...string) string {
	return `{"batches":[` + strings.Join(selections, ",") + `]}`
}

func decodeStatementCommit(t *testing.T, rec *httptest.ResponseRecorder) schemas.StatementCommitResponse {
	t.Helper()
	var envelope struct {
		Data schemas.StatementCommitResponse `json:"data"`
	}
	decodeOK(t, rec, "statement commit", &envelope)
	return envelope.Data
}

func batchesOf(t *testing.T, tx *gorm.DB, accountID string) []data.PointBatch {
	t.Helper()
	var batches []data.PointBatch
	if err := tx.Where("account_id = ?", accountID).Find(&batches).Error; err != nil {
		t.Fatalf("failed to read the batches: %v", err)
	}
	return batches
}

// The whole round trip: the credit the sync wrote becomes a batch, and sending the same
// selection again finds it already recorded instead of writing it twice.
func TestStatementCommit_RecordsTheCreditOnceOverHTTP(t *testing.T) {
	for _, program := range statementPrograms {
		t.Run(program.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				account, origin, agency := seedStatementTarget(t, tx)
				program.seedCredit(t, tx, account.ID, 168750)
				body := statementCommitBody(statementSelection(0, origin.ID, agency.ID, 978750))

				result := decodeStatementCommit(t, callStatementCommit(tx, program, account.ID, access.FullScope(), body))
				if result.Created != 1 || result.Credits != 1 || result.Skipped != 0 {
					t.Fatalf("first commit: want 1 created from 1 credit, got %+v", result)
				}
				batches := batchesOf(t, tx, account.ID)
				if len(batches) != 1 {
					t.Fatalf("want one batch, got %d", len(batches))
				}
				batch := batches[0]
				if batch.Points != 168750 || batch.CostCents != 978750 || batch.FundingAgencyID != agency.ID ||
					batch.OriginID == nil || *batch.OriginID != origin.ID || batch.AcquiredOn.Format(batchDateLayout) != "2026-09-04" {
					t.Errorf("batch: got %+v", batch)
				}

				result = decodeStatementCommit(t, callStatementCommit(tx, program, account.ID, access.FullScope(), body))
				if result.Created != 0 || result.Skipped != 1 {
					t.Errorf("same selection again: want 0 created and 1 skipped, got %+v", result)
				}
				if got := len(batchesOf(t, tx, account.ID)); got != 1 {
					t.Errorf("same selection again wrote a second batch: %d batches", got)
				}
			})
		})
	}
}

// The funding agency travels in the body, so the scope check is the handler's own: a caller
// may only have a credit paid for by one of its agencies, and a selection that names a
// foreign one refuses the whole commit before anything is written.
func TestStatementCommit_FundingAgencyMustBeInTheCallersScope(t *testing.T) {
	for _, program := range statementPrograms {
		t.Run(program.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				account, origin, agency := seedStatementTarget(t, tx)
				foreign := seedNamedAgency(t, tx, "Foreign")
				program.seedCredit(t, tx, account.ID, 5000)
				program.seedCredit(t, tx, account.ID, 7000)
				restricted := access.AgencyScope{IDs: []string{agency.ID}}

				rec := callStatementCommit(tx, program, account.ID, restricted,
					statementCommitBody(statementSelection(0, origin.ID, foreign.ID, 100)))
				if rec.Code != http.StatusNotFound {
					t.Errorf("foreign agency: got %d, want 404 (%s)", rec.Code, rec.Body.String())
				}

				// One good selection and one foreign: the foreign one sinks both.
				rec = callStatementCommit(tx, program, account.ID, restricted, statementCommitBody(
					statementSelection(0, origin.ID, agency.ID, 100),
					statementSelection(1, origin.ID, foreign.ID, 100),
				))
				if rec.Code != http.StatusNotFound {
					t.Errorf("one foreign selection among good ones: got %d, want 404 (%s)", rec.Code, rec.Body.String())
				}

				// A caller with no agency at all is refused the same way.
				rec = callStatementCommit(tx, program, account.ID, access.AgencyScope{},
					statementCommitBody(statementSelection(0, origin.ID, agency.ID, 100)))
				if rec.Code != http.StatusNotFound {
					t.Errorf("no agency in scope: got %d, want 404 (%s)", rec.Code, rec.Body.String())
				}

				if got := len(batchesOf(t, tx, account.ID)); got != 0 {
					t.Fatalf("a refused commit wrote %d batches", got)
				}

				result := decodeStatementCommit(t, callStatementCommit(tx, program, account.ID, restricted,
					statementCommitBody(statementSelection(0, origin.ID, agency.ID, 100))))
				if result.Created != 1 {
					t.Errorf("own agency: want 1 created, got %+v", result)
				}
			})
		})
	}
}

func TestStatementCommit_RefusesBadInput(t *testing.T) {
	for _, program := range statementPrograms {
		t.Run(program.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				account, origin, agency := seedStatementTarget(t, tx)
				program.seedCredit(t, tx, account.ID, 5000)
				plain := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: account.LoyaltyProgramID}
				create(t, tx, &plain)

				good := statementCommitBody(statementSelection(0, origin.ID, agency.ID, 100))
				tooMany := make([]string, statementMaxBatches+1)
				for index := range tooMany {
					tooMany[index] = statementSelection(0, origin.ID, agency.ID, 100)
				}

				cases := []struct {
					name      string
					accountID string
					body      string
					want      int
				}{
					{"unknown account", uuid.New().String(), good, http.StatusNotFound},
					{"account without its own inventory", plain.ID, good, http.StatusUnprocessableEntity},
					{"broken JSON", account.ID, `{"batches":`, http.StatusBadRequest},
					{"nothing selected", account.ID, `{"batches":[]}`, http.StatusBadRequest},
					{"origin that is not a uuid", account.ID, statementCommitBody(statementSelection(0, "nope", agency.ID, 100)), http.StatusBadRequest},
					{"negative cost", account.ID, statementCommitBody(statementSelection(0, origin.ID, agency.ID, -1)), http.StatusBadRequest},
					{"more selections than one commit takes", account.ID, statementCommitBody(tooMany...), http.StatusUnprocessableEntity},
				}
				for _, testCase := range cases {
					rec := callStatementCommit(tx, program, testCase.accountID, access.FullScope(), testCase.body)
					if rec.Code != testCase.want {
						t.Errorf("%s: got %d, want %d (%s)", testCase.name, rec.Code, testCase.want, rec.Body.String())
					}
				}
				if got := len(batchesOf(t, tx, account.ID)); got != 0 {
					t.Errorf("a refused commit wrote %d batches", got)
				}
			})
		})
	}
}

// An index the reconciliation does not have is not an error today: the selection is
// counted as skipped and nothing is written. Pinned so a change to that is deliberate.
func TestStatementCommit_IndexOutsideTheStatementIsSkipped(t *testing.T) {
	for _, program := range statementPrograms {
		t.Run(program.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				account, origin, agency := seedStatementTarget(t, tx)
				program.seedCredit(t, tx, account.ID, 5000)

				rec := callStatementCommit(tx, program, account.ID, access.FullScope(),
					statementCommitBody(statementSelection(7, origin.ID, agency.ID, 100)))
				result := decodeStatementCommit(t, rec)
				if result.Created != 0 || result.Skipped != 1 {
					t.Errorf("index out of range: want 0 created and 1 skipped, got %+v", result)
				}
				if got := len(batchesOf(t, tx, account.ID)); got != 0 {
					t.Errorf("an out-of-range index wrote %d batches", got)
				}
			})
		})
	}
}
