package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/carriers/traveos"
	"github.com/murandido/m-80-backend/internal/booking"
)

// probeHTTPTimeout is below the monitor's own probeTimeout so a slow bot loses
// the request rather than the whole probe budget.
const probeHTTPTimeout = 8 * time.Second

// TraveosProbe checks that the vendor answers an authenticated call.
//
// It calls the airline catalog rather than just authenticating, and that choice
// is load-bearing. The client caches its token, so a probe that only logged in
// would answer from the cache and report "up" while every real sync failed -
// which is close to the outage this whole feature exists because of. Hitting a
// real endpoint exercises the token, the vendor's routing and its database in
// one cheap read.
//
// A nil client means the process started without credentials, which cmd/api
// explicitly supports; that is not configured, not down. The parameter is the
// concrete *traveos.Client and not an interface on purpose: a nil *Client
// stored in an interface is not nil, and that check silently passing is a trap
// internal/bot already had to document once.
func TraveosProbe(client *traveos.Client) Probe {
	return func(ctx context.Context) (string, error) {
		if client == nil {
			return "", ErrNotConfigured
		}
		if _, err := client.ActiveAirlines(ctx); err != nil {
			return "", err
		}
		return "", nil
	}
}

// botStatusEnvelope is the shape cmd/bot's internal /status answers with. Only
// the session state is read: jid and pair_code are in that body too and must
// never travel any further - see handlers.SystemStatusHandler.
type botStatusEnvelope struct {
	Data struct {
		State string `json:"state"`
	} `json:"data"`
}

// BotProbe checks that the bot process is alive, and reports which state its
// WhatsApp session is in as the check's detail.
//
// Two facts, one call, because they fail independently: a bot whose process is
// gone answers nothing, and a bot that is running perfectly may still have been
// logged out of WhatsApp. Both are things the panel has to be able to say.
//
// It talks to BOT_INTERNAL_URL directly instead of going through the admin
// proxy in internal/api/v1/routes/bot.go: this runs on a background ticker with
// no request and no user, so there is no admin token to carry.
func BotProbe(internalURL string) Probe {
	client := &http.Client{Timeout: probeHTTPTimeout}

	return func(ctx context.Context) (string, error) {
		if internalURL == "" {
			return "", ErrNotConfigured
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, internalURL+"/status", nil)
		if err != nil {
			return "", fmt.Errorf("health: building bot status request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("health: bot status: %w", err)
		}
		defer closeBody(resp)

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("health: bot status returned %d", resp.StatusCode)
		}

		var envelope botStatusEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			// The process answered, so it is up. Only the session detail is
			// lost, and reporting the bot as down over a decoding problem
			// would point the operator at the wrong thing entirely.
			slog.Warn("health: could not decode the bot status body", "err", err)
			return "", nil
		}

		return envelope.Data.State, nil
	}
}

// closeBody drains and closes, logging instead of swallowing with `_ =`. The
// probe runs forever, so a leaked connection per tick is a leak that grows.
func closeBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	if err := resp.Body.Close(); err != nil {
		slog.Error("health: failed to close response body", "err", err)
	}
}

// BookingProviderProbe checks that an airline's own endpoint is answering.
//
// There is no cheap catalog call here the way there is for Traveos, so the probe
// asks about a booking that cannot exist and reads the shape of the refusal. A
// carrier that answers "no such locator" is a carrier that is up: the request
// reached its application. A carrier that answers an HTML block from its edge, or
// nothing at all, is not. That is the same control every measurement behind these
// integrations was made with, and it is the one signal available without spending
// somebody's real lookup.
//
// The provider's own Probe returns booking.ErrNotFound for the first case and
// booking.ErrUnavailable for the second, so this only has to ask which it was.
func BookingProviderProbe(prober booking.Prober) Probe {
	return func(ctx context.Context) (string, error) {
		if prober == nil {
			return "", ErrNotConfigured
		}
		if err := prober.Probe(ctx); !booking.Reached(err) {
			return "", err
		}
		return "", nil
	}
}
