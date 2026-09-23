package booking

import (
	"slices"
	"strings"
)

// MaxSurnameAttempts caps how many single words of a name a provider tries as the
// surname. Every attempt is a lookup at the airline, and only a refused one ever
// reaches the next.
const MaxSurnameAttempts = 3

// SurnameCandidates lists the single words of a name worth sending as an airline's
// surname field, in the order to try them.
//
// GOL and LATAM both match that field against the passengers' surnames and accept either
// a whole surname ("APARECIDA DA SILVA") or any single word of one ("SILVA"), but refuse a
// given name in front of it or a run of words that is not the whole surname ("DA SILVA").
// An issuance holds the operator's "SILVA" before its first sync and the passenger's full
// name after it, so single words are the one shape that works for both.
//
// The last word goes first because it is always part of the surname, which keeps the
// common case at one lookup. The rest follow right to left, for the names where it is not
// enough: a suffix the airline keeps apart, a typo in the last word. Particles are skipped
// - they are never a surname on their own.
func SurnameCandidates(name string) []string {
	words := strings.Fields(strings.ToUpper(name))
	if len(words) == 0 {
		return []string{""}
	}

	candidates := []string{words[len(words)-1]}
	for index := len(words) - 2; index >= 0 && len(candidates) < MaxSurnameAttempts; index-- {
		if !surnameParticles[words[index]] && !slices.Contains(candidates, words[index]) {
			candidates = append(candidates, words[index])
		}
	}
	return candidates
}

var surnameParticles = map[string]bool{"DA": true, "DE": true, "DO": true, "DAS": true, "DOS": true, "E": true}

// WholeSurnameCandidates lists what to send as the surname to a carrier that matches the
// field against a passenger's whole surname, ignoring spaces - "DE OLIVEIRA RODRIGUES"
// and "DEOLIVEIRARODRIGUES" both work - but refuses a single word of it ("RODRIGUES") and
// a given name in front of it. American and Avianca both do; GOL and LATAM do not, which
// is what SurnameCandidates is for.
//
// The name as stored goes first: before the first sync it is the surname the operator
// typed. After the sync the issuance holds the full name, "MARCO ANTONIO DE OLIVEIRA",
// so the trailing runs of words follow, shortest first, until one is the whole surname.
func WholeSurnameCandidates(name string, limit int) []string {
	words := strings.Fields(strings.ToUpper(name))
	if len(words) == 0 {
		return nil
	}
	candidates := []string{strings.Join(words, " ")}
	for start := len(words) - 1; start > 0 && len(candidates) < limit; start-- {
		run := strings.Join(words[start:], " ")
		if !slices.Contains(candidates, run) {
			candidates = append(candidates, run)
		}
	}
	return candidates
}
