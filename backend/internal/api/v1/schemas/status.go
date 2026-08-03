package schemas

// SystemStatus is what GET /status answers with: whether each dependency the
// panel relies on is reachable right now.
//
// Every field here is visible to *any* authenticated user, not just admins,
// which is the whole point of the route - the operator working the issuances
// grid is the one who needs to know the vendor is down. That also means this is
// the wrong place for anything sensitive; see DependencyStatus.
type SystemStatus struct {
	Bot     DependencyStatus `json:"bot"`
	Traveos DependencyStatus `json:"traveos"`
}

// DependencyStatus is one dependency's condition.
//
// What is deliberately NOT here: the bot's pair_code and jid. The pairing code
// is a credential - whoever reads it can pair the company's WhatsApp to their
// own phone - and it stays behind the admin-only proxy in
// internal/api/v1/routes/bot.go. Do not "complete the DTO" by adding them.
//
// Also absent: any vendor error message. Traveos answers nearly every failure
// with the same opaque 500 (see internal/traveos/errors.go), so a reason shown
// on screen would be invented. The real error goes to the log.
type DependencyStatus struct {
	// State is one of: up, down, not_configured, unknown. not_configured means
	// this server was never given the credentials, which is a different thing
	// from the dependency being broken.
	State string `json:"state" example:"up"`

	// Session is the WhatsApp connection state (connected, pairing,
	// disconnected) and is only ever set for the bot. A bot process can be up
	// and still be logged out of WhatsApp, so one field cannot carry both.
	Session string `json:"session,omitempty" example:"connected"`

	// CheckedAt is when the probe last ran, so the panel can say how stale the
	// answer is instead of implying it is live.
	CheckedAt string `json:"checked_at" example:"2026-08-02T14:32:07Z"`
}
