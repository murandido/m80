package handlers

import (
	"net/http/httputil"
	"net/url"
)

// NewBotProxy builds a pure reverse proxy to the bot's internal API
// (BOT_INTERNAL_URL) - no business logic, no parsing, just status code and
// body pass-through. Admin gating happens in the route, not here.
func NewBotProxy(botInternalURL string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(botInternalURL)
	if err != nil {
		return nil, err
	}

	return httputil.NewSingleHostReverseProxy(target), nil
}
