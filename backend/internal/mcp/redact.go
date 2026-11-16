package mcp

import "encoding/json"

// sensitiveKeys are dropped from every answer before it reaches the agent, wherever they are
// nested. The panel needs them - staff log into the programs with the account's login and
// password, and the ticket carries the passenger's document - but nothing the agent does needs
// either, and an issuance embeds its whole account, password included.
var sensitiveKeys = map[string]bool{
	"password": true,
	"login":    true,
	"document": true,
	"cpf":      true,
}

// redact removes sensitiveKeys from a JSON document. Something that does not decode is
// returned as is: it is not an object or list, so it has no keys to hide.
func redact(raw json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	cleaned, err := json.Marshal(redactValue(value))
	if err != nil {
		return raw
	}
	return cleaned
}

func redactValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, inner := range v {
			if sensitiveKeys[key] {
				delete(v, key)
				continue
			}
			v[key] = redactValue(inner)
		}
	case []any:
		for i, inner := range v {
			v[i] = redactValue(inner)
		}
	}
	return value
}
