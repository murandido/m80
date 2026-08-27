package promotion

import (
	"fmt"
	"net/url"
	"strings"
)

// CanonicalURLKey reduces a promotion link to the thing that identifies the campaign,
// so that the same campaign is recognised however it was shared.
//
// Promotion links reach a WhatsApp group after passing through newsletters, shortener
// redirects and forwards, and arrive carrying utm_source, gclid, fbclid and friends.
// Without this, the same campaign pasted twice looks like two campaigns and every CPF
// is enrolled a second time - which is exactly what the enrolment history exists to
// prevent, since the sites themselves cannot be asked whether a document is already in.
//
// The scheme is dropped rather than normalised: http and https of the same page are the
// same campaign, and so is a link that arrived without a scheme at all.
func CanonicalURLKey(raw string) (string, error) {
	parsed, err := parseLoosely(raw)
	if err != nil {
		return "", err
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	return HostOf(parsed) + path, nil
}

// HostOf returns the lowercased hostname without a leading "www.".
func HostOf(parsed *url.URL) string {
	host := strings.ToLower(parsed.Hostname())
	return strings.TrimPrefix(host, "www.")
}

// DomainOf is HostOf for a raw string, and is what a link is matched against when
// looking for its recipe.
func DomainOf(raw string) (string, error) {
	parsed, err := parseLoosely(raw)
	if err != nil {
		return "", err
	}
	return HostOf(parsed), nil
}

// parseLoosely accepts what people actually paste. A link copied out of WhatsApp often
// arrives without a scheme, and url.Parse reads "voeazul.com.br/ofertas" as a relative
// path with no host at all rather than failing, so the missing scheme has to be filled
// in before parsing rather than diagnosed after.
func parseLoosely(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("promotion: empty url")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("promotion: parsing %q: %w", raw, err)
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("promotion: %q has no host", raw)
	}
	return parsed, nil
}

// FindLink returns the first http(s) link in a message, and whether there was one.
// Only explicit schemes count: a bare "voeazul.com.br" mentioned in a sentence is
// somebody talking about the site, not asking for an enrolment run.
func FindLink(text string) (string, bool) {
	for _, field := range strings.Fields(text) {
		candidate := strings.Trim(field, ".,;:!?()[]{}<>\"'")
		if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
			if _, err := parseLoosely(candidate); err == nil {
				return candidate, true
			}
		}
	}
	return "", false
}
