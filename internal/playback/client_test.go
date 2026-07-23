package playback

import (
	"context"
	"errors"
	"testing"

	"traker/internal/domain"
)

type fakeProvider struct {
	configured bool
	link       Link
	err        error
	calls      int
}

func (provider *fakeProvider) Configured() bool { return provider.configured }

func (provider *fakeProvider) PlayLink(_ context.Context, _ domain.MediaRef) (Link, error) {
	provider.calls++
	return provider.link, provider.err
}

func TestClientUsesFirstSuccessfulProvider(t *testing.T) {
	first := &fakeProvider{configured: true, err: ErrNotFound}
	second := &fakeProvider{configured: true, link: Link{PlayURL: "https://plex.example/media", ServerName: "Plex"}}
	client := NewClient(first, second)

	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 278})
	if err != nil {
		t.Fatal(err)
	}
	if link.ServerName != "Plex" || first.calls != 1 || second.calls != 1 {
		t.Fatalf("unexpected result: link=%#v calls=%d/%d", link, first.calls, second.calls)
	}
}

func TestClientSkipsUnconfiguredProvidersAndReportsMissing(t *testing.T) {
	first := &fakeProvider{}
	second := &fakeProvider{configured: true, err: ErrNotFound}
	client := NewClient(first, second)
	if _, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if first.calls != 0 || second.calls != 1 {
		t.Fatalf("unexpected calls: %d/%d", first.calls, second.calls)
	}
}

func TestClientRejectsSeriesBeforeCallingProviders(t *testing.T) {
	provider := &fakeProvider{configured: true}
	client := NewClient(provider)
	if _, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tv", ID: 1}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected unsupported, got %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider was called %d times", provider.calls)
	}
}
