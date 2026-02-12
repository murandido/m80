package utils

import (
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// GetText extracts the text from any type of message.
func GetText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}

	if txt := msg.GetConversation(); txt != "" {
		return txt
	}

	if txt := msg.GetExtendedTextMessage().GetText(); txt != "" {
		return txt
	}

	if txt := msg.GetImageMessage().GetCaption(); txt != "" {
		return txt
	}

	return ""
}

// IsCommand returns if the string has the conditions to be a valid command
func IsCommand(msg string) bool {
	if strings.HasPrefix(msg, "!") {
		return true
	}

	return false
}
