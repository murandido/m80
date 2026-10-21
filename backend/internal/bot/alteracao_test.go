package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/gorm"
)

// correctionScenario is an emissaoScenario with one issuance already registered
// from a real EMISSAO message - which is where every correction starts. Building
// it by actually sending the message, rather than seeding a row, is what proves
// the two flows agree on what an issuance from WhatsApp looks like.
type correctionScenario struct {
	emissaoScenario

	Issuance data.Issuance
	Locator  string
}

func newCorrectionScenario(t *testing.T, tx *gorm.DB, airline data.Airline) correctionScenario {
	t.Helper()

	s := newEmissaoScenario(t, tx, airline)
	return correctionFrom(t, tx, s, s.text())
}

// correctionFrom is newCorrectionScenario for the tests whose base EMISSAO has to
// say something other than the default - an airline that demands a surname, for
// one, whose line 2 has to carry it before a correction can leave it out.
func correctionFrom(t *testing.T, tx *gorm.DB, s emissaoScenario, text string) correctionScenario {
	t.Helper()

	s.Bot.handleEmissao(context.Background(), s.event(), text, time.Now())

	locator := "PL" + s.Suffix
	var issuance data.Issuance
	if err := tx.Where("program_locator = ?", locator).First(&issuance).Error; err != nil {
		t.Fatalf("the base EMISSAO did not persist, so there is nothing to correct: %v", err)
	}

	// Forget the EMISSAO's own reaction and notification: every assertion below
	// is about what the correction did.
	s.Client.reset()
	s.Notify.Store(0)

	return correctionScenario{emissaoScenario: s, Issuance: issuance, Locator: locator}
}

// reset clears everything the fake recorded, so a test that has to send one
// message before the one it is about can still assert on exact counts.
func (f *fakeClientBase) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentReactions = nil
	f.sentTexts = nil
	f.sendCalls = 0
	f.markReadCalls = 0
}

// alteracao builds a correction message for this scenario's issuance.
func (s correctionScenario) alteracao(corrections ...string) string {
	return strings.Join(append([]string{"ALTERACAO - " + s.Locator}, corrections...), "\n")
}

func (s correctionScenario) reload(t *testing.T, tx *gorm.DB) data.Issuance {
	t.Helper()

	var issuance data.Issuance
	if err := tx.First(&issuance, "id = ?", s.Issuance.ID).Error; err != nil {
		t.Fatalf("failed to reload the issuance: %v", err)
	}
	return issuance
}

// auditSnapshot is the shape applyCorrection writes into audit_logs.details.
type auditSnapshot struct {
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
}

func updateAudit(t *testing.T, tx *gorm.DB, issuanceID string) (auditSnapshot, bool) {
	t.Helper()

	var entries []data.AuditLog
	if err := tx.Where("issuance_id = ? AND action = ?", issuanceID, auditActionUpdated).
		Order("created_at").Find(&entries).Error; err != nil {
		t.Fatalf("failed to read audit_logs: %v", err)
	}
	if len(entries) == 0 {
		return auditSnapshot{}, false
	}
	if len(entries) > 1 {
		t.Fatalf("found %d update entries, want at most one per correction", len(entries))
	}

	var snapshot auditSnapshot
	if err := json.Unmarshal(entries[0].Details, &snapshot); err != nil {
		t.Fatalf("audit_logs.details is not the expected before/after shape: %v", err)
	}
	return snapshot, true
}

func assertRocket(t *testing.T, s correctionScenario) {
	t.Helper()

	if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
		t.Fatalf("reactions = %v, want a single 🚀", reactions)
	}
	// The reaction itself goes out as a send, so one send means the reaction and
	// nothing else - the only way to catch a confirmation line creeping back in.
	if sends, _ := s.Client.calls(); sends != 1 {
		t.Fatalf("the bot sent %d message(s) on a successful correction, want 1", sends)
	}
	if got := s.Updated.Load(); got != 1 {
		t.Fatalf("notifyIssuanceUpdated called %d time(s), want 1", got)
	}
}

func assertRefused(t *testing.T, s correctionScenario, mentions string) {
	t.Helper()

	if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
		t.Fatalf("reactions = %v, want a single ❌", reactions)
	}
	if got := s.Updated.Load(); got != 0 {
		t.Fatalf("notifyIssuanceUpdated called %d time(s) for a refused message, want 0", got)
	}
	if mentions != "" && !strings.Contains(s.Client.lastText(), mentions) {
		t.Fatalf("the refusal %q does not mention %q", s.Client.lastText(), mentions)
	}
}

// assertRocketWithoutChange covers a message the bot understood and had nothing
// to do about. It reacts like any other success - silence would read as a bot
// that is down - but there is nothing to broadcast, so no SSE event follows.
func assertRocketWithoutChange(t *testing.T, s correctionScenario) {
	t.Helper()

	if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
		t.Fatalf("reactions = %v, want a single 🚀", reactions)
	}
	if sends, _ := s.Client.calls(); sends != 1 {
		t.Fatalf("the bot sent %d message(s), want only the reaction", sends)
	}
	if got := s.Updated.Load(); got != 0 {
		t.Fatalf("notifyIssuanceUpdated called %d time(s) when nothing changed, want 0", got)
	}
}

// assertSilence is for the messages the bot must never let on that it saw at all:
// a chat nobody approved, and ordinary conversation.
func assertSilence(t *testing.T, s correctionScenario) {
	t.Helper()

	if sends, _ := s.Client.calls(); sends != 0 {
		t.Fatalf("the bot sent %d message(s), want none at all", sends)
	}
	if got := s.Updated.Load(); got != 0 {
		t.Fatalf("notifyIssuanceUpdated called %d time(s) when nothing changed, want 0", got)
	}
}

func TestHandleAlteracao_CorrectsTheAmounts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("50k * 33,00 + 100 tx = 1.750,00"), time.Now())

		corrected := s.reload(t, tx)
		if corrected.Points != 50000 {
			t.Fatalf("points = %d, want 50000", corrected.Points)
		}
		if corrected.ValueOfTheThousand != 3300 {
			t.Fatalf("value_of_the_thousand = %d, want 3300", corrected.ValueOfTheThousand)
		}
		if corrected.Taxes == nil || *corrected.Taxes != 10000 {
			t.Fatalf("taxes = %v, want 10000", corrected.Taxes)
		}

		assertRocket(t, s)

		snapshot, logged := updateAudit(t, tx, s.Issuance.ID)
		if !logged {
			t.Fatal("a correction that changed the price left no audit trail")
		}
		if snapshot.Before["points"] != float64(31000) || snapshot.After["points"] != float64(50000) {
			t.Fatalf("audit details for points = %v -> %v, want 31000 -> 50000",
				snapshot.Before["points"], snapshot.After["points"])
		}
		// Only what moved: a history entry restating every untouched column
		// buries the one that matters.
		if _, present := snapshot.After["account_id"]; present {
			t.Fatal("the audit trail lists account_id, which this message never touched")
		}
	})
}

func TestHandleAlteracao_ResolvesTheAccountAndAdministrator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("ContaNova"+s.Suffix+" - Wilmara"+s.Suffix), time.Now())

		assertRocket(t, s)

		corrected := s.reload(t, tx)
		if corrected.AccountID == s.Issuance.AccountID {
			t.Fatal("the issuance still points at the old account")
		}

		// The new alias conjures the whole chain, exactly as an EMISSAO would.
		var account data.Account
		if err := tx.First(&account, "id = ?", corrected.AccountID).Error; err != nil {
			t.Fatalf("the account named by the correction does not exist: %v", err)
		}
		if account.Complete {
			t.Fatal("an account conjured from a correction must be marked incomplete")
		}
		if account.AdministratorID == nil {
			t.Fatal("the administrator named on the line was not linked")
		}
	})
}

func TestHandleAlteracao_ChangesTheObservation(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		preset := data.ObservationPreset{Code: "ACORDO" + s.Suffix, Text: "Acordo comercial"}
		seed(t, tx, &preset)

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao(preset.Code), time.Now())

		assertRocket(t, s)

		corrected := s.reload(t, tx)
		if corrected.Observation == nil || *corrected.Observation != preset.Text {
			t.Fatalf("observation = %v, want the preset text %q", corrected.Observation, preset.Text)
		}
	})
}

// A bare word that matches no preset is an account alias, not a bad observation
// code - the two shapes are identical and only the database can tell them apart.
func TestHandleAlteracao_ABareWordThatIsNoPresetIsAnAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("OutraConta"+s.Suffix), time.Now())

		assertRocket(t, s)

		corrected := s.reload(t, tx)
		if corrected.AccountID == s.Issuance.AccountID {
			t.Fatal("the bare word was not read as an account alias")
		}
		if corrected.Observation != nil {
			t.Fatalf("observation = %v, want it left alone", *corrected.Observation)
		}
	})
}

func TestHandleAlteracao_ChangesTheRoute(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})
		seedAirport(t, tx, "GRU", "Guarulhos")

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("GRU - SILVA"), time.Now())

		assertRocket(t, s)

		corrected := s.reload(t, tx)
		if corrected.OriginIATACode == nil || *corrected.OriginIATACode != "GRU" {
			t.Fatalf("origin_iata_code = %v, want GRU", corrected.OriginIATACode)
		}
		if corrected.MainPassengerName == nil || *corrected.MainPassengerName != "SILVA" {
			t.Fatalf("main_passenger_name = %v, want SILVA", corrected.MainPassengerName)
		}
	})
}

// "Bia" and "GRU" are the same shape. Only the airports table decides, and an
// alias that is not an airport must not be mistaken for one.
func TestHandleAlteracao_AThreeLetterAliasIsNotAnAirport(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		account := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: s.Program.ID, Complete: true}
		seed(t, tx, &account)
		seed(t, tx, &data.AccountNickname{
			AccountID:        account.ID,
			LoyaltyProgramID: s.Program.ID,
			Nickname:         "Bia",
		})

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("Bia"), time.Now())

		assertRocket(t, s)

		corrected := s.reload(t, tx)
		if corrected.AccountID != account.ID {
			t.Fatal("the three-letter alias did not resolve to the account it names")
		}
		if corrected.OriginIATACode == nil || *corrected.OriginIATACode != "CNF" {
			t.Fatalf("origin_iata_code = %v, want the untouched CNF", corrected.OriginIATACode)
		}
	})
}

// Whichever order the operator thought of the two corrections in, they land the
// same way - the whole point of classifying by shape rather than position.
func TestHandleAlteracao_LineOrderDoesNotMatter(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})
		seedAirport(t, tx, "GRU", "Guarulhos")

		s.Bot.handleAlteracao(context.Background(), s.event(),
			s.alteracao("GRU", "50k * 33,00 + 100 tx = 1.750,00"), time.Now())

		assertRocket(t, s)

		corrected := s.reload(t, tx)
		if corrected.Points != 50000 {
			t.Fatalf("points = %d, want 50000", corrected.Points)
		}
		if corrected.OriginIATACode == nil || *corrected.OriginIATACode != "GRU" {
			t.Fatalf("origin_iata_code = %v, want GRU", corrected.OriginIATACode)
		}
	})
}

// The route line of an ALTERACAO names only what is changing, so an airline that
// demands a surname on an EMISSAO must not turn "fix the origin" into "retype the
// passenger". Refusing the line over a field the message never claimed to restate
// is what made correcting a GOL issuance's origin impossible.
func TestHandleAlteracao_CorrectsTheOriginAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		base := newEmissaoScenario(t, tx, data.Airline{RequiresLastName: true})
		seedAirport(t, tx, "GRU", "Guarulhos")

		// The airline demands the surname of the EMISSAO, so the base message
		// carries it - and the correction below is what leaves it out.
		s := correctionFrom(t, tx, base, strings.Replace(base.text(), "\nCNF\n", "\nCNF - SILVA\n", 1))

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("GRU"), time.Now())

		assertRocket(t, s)

		corrected := s.reload(t, tx)
		if origin := corrected.OriginIATACode; origin == nil || *origin != "GRU" {
			t.Fatalf("origin_iata_code = %v, want GRU", origin)
		}
		if name := corrected.MainPassengerName; name == nil || *name != "SILVA" {
			t.Fatalf("main_passenger_name = %v, want SILVA left as it was", name)
		}
	})
}

// A message that says nothing new is not an error and it is not a change either.
// It still earns the reaction - from the group's side, no reaction at all reads
// as a bot that is down - but there is nothing to record or broadcast.
func TestHandleAlteracao_ARepeatedMessageChangesNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("31k * 16,5 + 189,22 tx = 700,72"), time.Now())

		assertRocketWithoutChange(t, s)
		if _, logged := updateAudit(t, tx, s.Issuance.ID); logged {
			t.Fatal("a correction that changed nothing still wrote to the history")
		}
	})
}

// Once an issuance is complete its route has been confirmed against the source,
// and retyping it from memory must not overwrite that. Asking anyway is refused
// out loud: a route line in an ALTERACAO is deliberate, so applying the rest and
// reacting 🚀 would tell the operator the origin moved when it did not.
func TestHandleAlteracao_RefusesARouteOnACompleteIssuance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})
		seedAirport(t, tx, "GRU", "Guarulhos")

		if err := tx.Model(&data.Issuance{}).Where("id = ?", s.Issuance.ID).
			Update("complete", true).Error; err != nil {
			t.Fatalf("failed to complete the issuance: %v", err)
		}

		t.Run("route alone is refused", func(t *testing.T) {
			s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("GRU"), time.Now())

			assertRefused(t, s, "já está completa")
			if origin := s.reload(t, tx).OriginIATACode; origin == nil || *origin != "CNF" {
				t.Fatalf("origin_iata_code = %v, want the confirmed CNF", origin)
			}
		})

		t.Run("the rest of the message goes down with it", func(t *testing.T) {
			s.Client.reset()
			s.Updated.Store(0)

			s.Bot.handleAlteracao(context.Background(), s.event(),
				s.alteracao("GRU", "50k * 33,00 + 100 tx = 1.750,00"), time.Now())

			assertRefused(t, s, "já está completa")

			corrected := s.reload(t, tx)
			if corrected.Points != 31000 {
				t.Fatalf("points = %d, want the message refused whole, leaving 31000", corrected.Points)
			}
			if origin := corrected.OriginIATACode; origin == nil || *origin != "CNF" {
				t.Fatalf("origin_iata_code = %v, want the confirmed CNF", origin)
			}
		})
	})
}

func TestHandleAlteracao_Refusals(t *testing.T) {
	cases := []struct {
		name     string
		message  func(s correctionScenario) string
		mentions string
	}{
		{
			name:     "locator nobody issued",
			message:  func(correctionScenario) string { return "ALTERACAO - NAOEXISTE\nBia" },
			mentions: "Não encontrei nenhuma emissão",
		},
		{
			name: "the numbers do not add up",
			message: func(s correctionScenario) string {
				return s.alteracao("50k * 33,00 + 100 tx = 999,99")
			},
			mentions: "não fecham",
		},
		{
			name: "two lines saying the same thing",
			message: func(s correctionScenario) string {
				return s.alteracao("50k * 33,00 + 100 tx = 1.750,00", "60k * 33,00 + 100 tx = 2.080,00")
			},
			mentions: "duas linhas de valores",
		},
		{
			name:     "nothing to correct",
			message:  func(s correctionScenario) string { return s.alteracao() },
			mentions: "pelo menos uma linha",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				s := newCorrectionScenario(t, tx, data.Airline{})

				s.Bot.handleAlteracao(context.Background(), s.event(), tt.message(s), time.Now())

				assertRefused(t, s, tt.mentions)
				if s.reload(t, tx).Points != 31000 {
					t.Fatal("a refused correction still changed the issuance")
				}
			})
		})
	}
}

func TestHandleAlteracao_ACancelledIssuanceIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		now := time.Now()
		if err := tx.Model(&data.Issuance{}).Where("id = ?", s.Issuance.ID).
			Update("cancelled_at", now).Error; err != nil {
			t.Fatalf("failed to cancel the issuance: %v", err)
		}

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("50k * 33,00 + 100 tx = 1.750,00"), time.Now())

		assertRefused(t, s, "está cancelada")
		if s.reload(t, tx).Points != 31000 {
			t.Fatal("a cancelled issuance was corrected anyway")
		}
	})
}

// The same silence an EMISSAO gets: someone typing in a group the bot merely
// happens to be in must not discover that it reads.
func TestHandleAlteracao_IgnoresChatsThatWereNotApproved(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		if err := tx.Model(&data.ListenedChat{}).Where("chat_jid = ?", s.ChatJID).
			Update("active", false).Error; err != nil {
			t.Fatalf("failed to stop monitoring the chat: %v", err)
		}

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("50k * 33,00 + 100 tx = 1.750,00"), time.Now())

		assertSilence(t, s)
		if s.reload(t, tx).Points != 31000 {
			t.Fatal("the bot corrected an issuance from a chat it may not act on")
		}
	})
}

// Resolution creates rows before the write that may still fail. None of them may
// survive it.
func TestHandleAlteracao_RollsBackAutoCreatedRecords(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		// The account line resolves first and conjures an account; the amounts
		// line then fails the checksum.
		s.Bot.handleAlteracao(context.Background(), s.event(),
			s.alteracao("ContaFantasma"+s.Suffix, "50k * 33,00 + 100 tx = 999,99"), time.Now())

		assertRefused(t, s, "não fecham")

		var nicknames int64
		if err := tx.Model(&data.AccountNickname{}).
			Where("nickname = ?", "ContaFantasma"+s.Suffix).Count(&nicknames).Error; err != nil {
			t.Fatalf("failed to count account_nicknames: %v", err)
		}
		if nicknames != 0 {
			t.Fatal("the account conjured from the alias survived a rolled-back correction")
		}
	})
}

func seedAirport(t *testing.T, tx *gorm.DB, code, name string) {
	t.Helper()

	var airport data.Airport
	if err := tx.Where(data.Airport{IATACode: code}).
		Attrs(data.Airport{Name: name, Country: "BR"}).
		FirstOrCreate(&airport).Error; err != nil {
		t.Fatalf("failed to seed the %s airport: %v", code, err)
	}
}

// A correction to the price reaches the sale: one settled by hand at the old price stays
// settled at the new one, instead of reading as half paid.
func TestHandleAlteracao_RepricingReachesTheSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		var found data.Sale
		if err := tx.First(&found, "issuance_id = ?", s.Issuance.ID).Error; err != nil {
			t.Fatalf("the EMISSAO left no sale: %v", err)
		}
		amount, err := sale.AmountOf(tx, found.ID)
		if err != nil {
			t.Fatalf("failed to read the price: %v", err)
		}
		payment := data.SalePayment{SaleID: found.ID, AmountCents: amount.Cents, PaidOn: time.Now(), CreatedByID: s.Issuance.IssuerID}
		if err := tx.Create(&payment).Error; err != nil {
			t.Fatalf("failed to pay the sale: %v", err)
		}
		if err := sale.Recalculate(tx, found.ID); err != nil {
			t.Fatalf("failed to settle the sale: %v", err)
		}

		s.Bot.handleAlteracao(context.Background(), s.event(), s.alteracao("50k * 33,00 + 100 tx = 1.750,00"), time.Now())

		if err := tx.First(&found, "id = ?", found.ID).Error; err != nil {
			t.Fatalf("failed to reload the sale: %v", err)
		}
		if !found.Paid {
			t.Error("a sale paid by hand should stay paid after the price went up")
		}
		repriced, err := sale.AmountOf(tx, found.ID)
		if err != nil {
			t.Fatalf("failed to read the new price: %v", err)
		}
		if paid, err := sale.PaidCents(tx, found.ID); err != nil || paid != repriced.Cents {
			t.Errorf("paid: got %d, want the new price %d (err %v)", paid, repriced.Cents, err)
		}
	})
}
