package bot

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestGetText(t *testing.T) {
	// test table
	tests := []struct {
		name     string
		input    *waE2E.Message
		expected string
	}{
		{
			name:     "nil message",
			input:    nil,
			expected: "",
		},
		{
			name: "extract Conversation (simple text)",
			input: &waE2E.Message{
				Conversation: proto.String("Hello World"),
			},
			expected: "Hello World",
		},
		{
			name: "extract ExtendedTextMessage (formated text)",
			input: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: proto.String("Hello World (ExtendedTextMessage)"),
				},
			},
			expected: "Hello World (ExtendedTextMessage)",
		},
		{
			name: "extract by prioritizing Conversation (instead of ExtendedTextMessage)",
			input: &waE2E.Message{
				Conversation: proto.String("Hello World (Priority)"),
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: proto.String("Ignorado (ExtendedTextMessage)"),
				},
			},
			expected: "Hello World (Priority)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetText(tt.input)

			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsCommand(t *testing.T) {
	// test table
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty",
			input:    "",
			expected: false,
		},
		{
			name:     "valid command",
			input:    "!ping",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCommand(tt.input)

			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}
