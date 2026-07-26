package bot

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestSenderPhone_PNMode_StripsCountryCode(t *testing.T) {
	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			AddressingMode: types.AddressingModePN,
			Sender:         types.NewJID("5511987654321", types.DefaultUserServer),
		},
	}
	if got := senderPhone(info); got != "11987654321" {
		t.Fatalf("senderPhone: got %q, want %q", got, "11987654321")
	}
}

func TestSenderPhone_AlreadyLocal_NoCountryCodeToStrip(t *testing.T) {
	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			AddressingMode: types.AddressingModePN,
			Sender:         types.NewJID("11987654321", types.DefaultUserServer),
		},
	}
	if got := senderPhone(info); got != "11987654321" {
		t.Fatalf("senderPhone: got %q, want %q", got, "11987654321")
	}
}

// LID addressing: Sender is a hidden-user JID (not phone-bearing), the real
// phone lives in SenderAlt instead.
func TestSenderPhone_LIDMode_UsesSenderAlt(t *testing.T) {
	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			AddressingMode: types.AddressingModeLID,
			Sender:         types.NewJID("123456789012345", types.HiddenUserServer),
			SenderAlt:      types.NewJID("5511987654321", types.DefaultUserServer),
		},
	}
	if got := senderPhone(info); got != "11987654321" {
		t.Fatalf("senderPhone: got %q, want %q", got, "11987654321")
	}
}

// If SenderAlt isn't phone-bearing either (unexpected, but shouldn't crash),
// fall back to whatever Sender.User holds rather than picking SenderAlt blindly.
func TestSenderPhone_LIDMode_NoUsablePhoneAltFallsBackToSender(t *testing.T) {
	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			AddressingMode: types.AddressingModeLID,
			Sender:         types.NewJID("123456789012345", types.HiddenUserServer),
		},
	}
	if got := senderPhone(info); got != "123456789012345" {
		t.Fatalf("senderPhone: got %q, want %q", got, "123456789012345")
	}
}

// Regression test for the reported bug: WhatsApp's JID for this number
// arrives without the optional mobile "nono dígito" (1198887777) even though
// the user's real number has it (11998887777) - senderPhone must normalize
// so it matches whatever canonical form users.phone was saved in.
func TestSenderPhone_PNMode_MissingNinthDigit_GetsNormalized(t *testing.T) {
	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			AddressingMode: types.AddressingModePN,
			Sender:         types.NewJID("551198887777", types.DefaultUserServer),
		},
	}
	if got := senderPhone(info); got != "11998887777" {
		t.Fatalf("senderPhone: got %q, want %q", got, "11998887777")
	}
}

func TestSenderPhone_LIDMode_MissingNinthDigit_GetsNormalized(t *testing.T) {
	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			AddressingMode: types.AddressingModeLID,
			Sender:         types.NewJID("123456789012345", types.HiddenUserServer),
			SenderAlt:      types.NewJID("551198887777", types.DefaultUserServer),
		},
	}
	if got := senderPhone(info); got != "11998887777" {
		t.Fatalf("senderPhone: got %q, want %q", got, "11998887777")
	}
}
