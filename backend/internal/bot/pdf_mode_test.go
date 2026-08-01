package bot

import (
	"context"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"gorm.io/gorm"
)

func TestParsePDFArgument(t *testing.T) {
	// Pure, so it runs without Postgres.
	tests := []struct {
		name   string
		text   string
		wantOn bool
		wantOK bool
	}{
		{"on", "!pdf on", true, true},
		{"off", "!pdf off", false, true},
		{"portuguese on", "!pdf ligar", true, true},
		{"portuguese off", "!pdf desativar", false, true},
		{"mixed case", "!PDF On", true, true},
		{"extra whitespace", "  !pdf   off  ", false, true},

		// No argument is a question about the current state, not a command - a
		// blind toggle in a group nobody can see the state of is wrong half the
		// time, and the wrong half either stops files the team is waiting for or
		// starts posting documents into a conversation with a client in it.
		{"bare", "!pdf", false, false},

		// Unreadable rather than guessed at.
		{"unknown argument", "!pdf sim", false, false},
		{"nonsense", "!pdf talvez amanha", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			on, ok := parsePDFArgument(tc.text)
			if on != tc.wantOn || ok != tc.wantOK {
				t.Errorf("parsePDFArgument(%q) = (%v, %v), want (%v, %v)", tc.text, on, ok, tc.wantOn, tc.wantOK)
			}
		})
	}
}

func TestPDFCommandIsAdmittedByIntake(t *testing.T) {
	// Both ends are needed: the intake gate only admits a known command, and the
	// router only answers one it has a case for. Miss the first and the message is
	// ignored; miss the second and the 🕐 hangs on screen forever.
	if !isKnownCommand("!pdf on") {
		t.Fatal("!pdf is not in the known-command list, so intake would ignore it")
	}
}

func TestCmdPDFTurnsTheModeOnAndOff(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		s.Bot.cmdPDF(context.Background(), s.event(), "!pdf on", time.Now())
		if !pdfModeOf(t, tx, s.ChatJID) {
			t.Fatal("pdf_mode is still off after !pdf on")
		}

		s.Bot.cmdPDF(context.Background(), s.eventWithID("second"), "!pdf off", time.Now())
		if pdfModeOf(t, tx, s.ChatJID) {
			t.Fatal("pdf_mode is still on after !pdf off")
		}

		// Text as well as the 🚀, which only !ping does otherwise: an emoji cannot
		// say "on" or "off", and the 🚀 is what closes the clock intake opened.
		if text := s.Client.lastText(); text == "" {
			t.Fatal("the bot answered without text; an emoji cannot say \"on\" or \"off\"")
		}
		if reactions := s.Client.reactions(); len(reactions) != 2 || reactions[1] != "🚀" {
			t.Errorf("reactions = %v, want a 🚀 per command", reactions)
		}
	})
}

func TestCmdPDFWithNoArgumentReportsTheStateWithoutChangingIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		s.Bot.cmdPDF(context.Background(), s.event(), "!pdf on", time.Now())

		s.Bot.cmdPDF(context.Background(), s.eventWithID("query"), "!pdf", time.Now())

		if !pdfModeOf(t, tx, s.ChatJID) {
			t.Fatal("a bare !pdf changed the state; it must only report it")
		}
	})
}

func TestCmdPDFRefusesAChatTheBotDoesNotMonitor(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		// Approved once, stopped since. There is no EMISSAO here to attach a PDF
		// to, so the setting would be meaningless.
		if err := tx.Model(&data.ListenedChat{}).Where("chat_jid = ?", s.ChatJID).
			Update("active", false).Error; err != nil {
			t.Fatalf("failed to stop the chat: %v", err)
		}

		s.Bot.cmdPDF(context.Background(), s.event(), "!pdf on", time.Now())

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
			t.Fatalf("reactions = %v, want a single ❌", reactions)
		}
		if pdfModeOf(t, tx, s.ChatJID) {
			t.Error("pdf_mode was written for a chat the bot does not monitor")
		}
	})
}

func TestSendTicketPDFDoesNothingWhenTheModeIsOff(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// Off is the default, and it has to stay a real gate: the bot posting
		// documents into a group that never asked is its own kind of failure.
		s := newEmissaoScenario(t, tx, data.Airline{})

		s.Bot.sendTicketPDF(context.Background(), s.event(), "any-issuance")

		if got := s.Client.uploadCalls; got != 0 {
			t.Errorf("uploaded %d file(s) with the mode off, want 0", got)
		}
	})
}

func TestSendDocumentCarriesTheUploadKeysAndTheFileName(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		err := s.Bot.SendDocument(context.Background(), s.event(), []byte("%PDF-1.7 fake"), "bilhete-ABC123.pdf", "application/pdf")
		if err != nil {
			t.Fatalf("SendDocument: %v", err)
		}

		if got := s.Client.uploadCalls; got != 1 {
			t.Fatalf("uploaded %d time(s), want 1 - WhatsApp does not accept file bytes inside a message", got)
		}

		document := s.Client.lastDocument()
		if document == nil {
			t.Fatal("no DocumentMessage was sent")
		}
		// Every one of these comes from the upload response, and dropping any one
		// of them leaves the recipient with a document that will not open.
		if document.GetURL() == "" || document.GetDirectPath() == "" {
			t.Error("the message is missing the media location from the upload")
		}
		if document.GetFileLength() == 0 {
			t.Error("the message reports a zero-length file")
		}
		if document.GetFileName() != "bilhete-ABC123.pdf" {
			t.Errorf("file name = %q, want the locator-based one", document.GetFileName())
		}
		if document.GetMimetype() != "application/pdf" {
			t.Errorf("mime type = %q, want application/pdf", document.GetMimetype())
		}
		// Quoted, so the file lands attached to the EMISSAO it belongs to rather
		// than floating loose in a busy group.
		if document.GetContextInfo().GetStanzaID() != s.event().Info.ID {
			t.Error("the document does not quote the message that produced it")
		}
	})
}

func TestHandleEmissaoDoesNotSendAPDFWhenTheModeIsOff(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, traveosAirline())
		s.Bot.Traveos = workingTraveos()

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		if got := s.Client.uploadCalls; got != 0 {
			t.Errorf("uploaded %d file(s) into a chat that never asked for them, want 0", got)
		}
	})
}

func TestAFailingPDFDoesNotChangeTheAnswerToTheGroup(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// The group has already been told the emission worked by the time the PDF is
		// even attempted. PDF_SERVICE_URL points at nothing in the test container, so
		// this exercises the real failure: the render or the upload gives up, and the
		// 🚀 stands.
		s := newEmissaoScenario(t, tx, traveosAirline())
		s.Bot.Traveos = workingTraveos()
		s.Bot.cmdPDF(context.Background(), s.eventWithID("enable"), "!pdf on", time.Now())

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		reactions := s.Client.reactions()
		if len(reactions) == 0 || reactions[len(reactions)-1] != "🚀" {
			t.Fatalf("reactions = %v, want the emission to still end in 🚀", reactions)
		}

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("the issuance did not survive a failed pdf: %v", err)
		}
		if !issuance.Complete {
			t.Error("the issuance lost its completion because the pdf failed")
		}
	})
}

func pdfModeOf(t *testing.T, tx *gorm.DB, chatJID string) bool {
	t.Helper()

	var chat data.ListenedChat
	if err := tx.First(&chat, "chat_jid = ?", chatJID).Error; err != nil {
		t.Fatalf("failed to read listened_chats for %s: %v", chatJID, err)
	}
	return chat.PDFMode
}

// lastDocument returns the DocumentMessage from the most recent send, or nil if
// the last thing sent was not one.
func (f *fakeClientBase) lastDocument() *waE2E.DocumentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastDocumentMessage
}
