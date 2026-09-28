package booking

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type stubProvider struct{ name string }

func (s stubProvider) Name() string { return s.name }
func (s stubProvider) Fetch(context.Context, Request) (*Booking, error) {
	return nil, ErrNotFound
}

func TestRegistryMatchesAirlineCodesRegardlessOfCase(t *testing.T) {
	registry := NewRegistry(stubProvider{name: "traveos"})
	registry.Register("azul", stubProvider{name: "azul"})

	// The code arrives from airlines.code on one side and from a hand-written
	// wiring line on the other; neither is worth trusting for case or padding.
	for _, code := range []string{"AZUL", "azul", " Azul "} {
		provider, ok := registry.Direct(code)
		if !ok {
			t.Fatalf("no direct provider for %q", code)
		}
		if provider.Name() != "azul" {
			t.Fatalf("got provider %q for %q", provider.Name(), code)
		}
	}

	if _, ok := registry.Direct("LATAM"); ok {
		t.Fatal("an unregistered airline must not resolve to a direct provider")
	}
	if registry.Fallback().Name() != "traveos" {
		t.Fatal("the fallback is not the one the registry was built with")
	}
}

func TestReachedSeparatesAnAnswerFromSilence(t *testing.T) {
	// A carrier that correctly rejects a made-up locator is a carrier that is up:
	// this is the whole basis of the synthetic probe, so it gets a test of its own.
	for _, err := range []error{nil, ErrNotFound, NotFound("locator %s", "ZZZZZZ")} {
		if !Reached(err) {
			t.Fatalf("expected %v to count as the provider having answered", err)
		}
	}

	for _, err := range []error{ErrUnavailable, Unavailable(nil, "edge returned 403"), errors.New("boom")} {
		if Reached(err) {
			t.Fatalf("expected %v to count as the provider not having answered", err)
		}
	}
}

func TestUnavailableKeepsBothTheSentinelAndTheCause(t *testing.T) {
	// The cause has to stay reachable: the issuance handler answers some of the
	// vendor's own sentinels with their own status, and losing the chain would
	// quietly demote all of them to a generic 502.
	cause := errors.New("traveos: token rejected")
	err := Unavailable(cause, "searching for %s", "ABC123")

	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("Unavailable must wrap ErrUnavailable")
	}
	if !errors.Is(err, cause) {
		t.Fatal("Unavailable must keep the cause in the chain")
	}
	if got := fmt.Sprint(err); got != "booking: the provider could not be reached: searching for ABC123: traveos: token rejected" {
		t.Fatalf("unexpected message: %s", got)
	}
}
