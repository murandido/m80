package bot

import (
	"errors"
	"strings"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// resolveSenderUser resolves the sender of an incoming message to an
// internal users row, matching on phone number. Returns (nil, nil) when the
// sender isn't a known user - not an error, just a fact the caller reacts to
// (e.g. by replying with a "not registered" reaction rather than crashing).
func (b *Bot) resolveSenderUser(evt *events.Message) (*data.User, error) {
	phone := senderPhone(evt.Info)

	var user data.User
	err := b.DB.First(&user, "phone = ?", phone).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// senderPhone extracts the sender's phone number, normalized to the same
// canonical local BR format (no country code, nono dígito resolved per
// utils.NormalizeBRPhone) that users.phone is stored in - see that
// function's own doc comment for the create/update side of this.
//
// Two things make this non-trivial:
//
//  1. LID addressing. When Info.AddressingMode is "lid", Info.Sender is a
//     hidden-user/LID JID, not a phone number - the phone-bearing JID is in
//     Info.SenderAlt instead. Checked by JID server rather than trusting
//     AddressingMode blindly, so this is correct either way.
//  2. Country code. JID.User carries the full international number (e.g.
//     "5511987654321"), but users.phone is the local 10-11 digit format with
//     no "55" prefix (see utils.ValidateBRPhone and the login schema). The
//     "55" prefix is only stripped when what's left is a plausible
//     local-length number, to avoid mangling a JID that doesn't actually
//     carry the country code.
func senderPhone(info types.MessageInfo) string {
	phoneJID := info.Sender
	if phoneJID.Server != types.DefaultUserServer && info.SenderAlt.Server == types.DefaultUserServer {
		phoneJID = info.SenderAlt
	}

	digits := utils.ClearPhone(phoneJID.User)
	if strings.HasPrefix(digits, "55") {
		if rest := digits[2:]; len(rest) == 10 || len(rest) == 11 {
			digits = rest
		}
	}
	return utils.NormalizeBRPhone(digits)
}
