package emby

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"traker/internal/domain"
)

func TestPlayLinkQueriesServersInOrderAndResolvesMovieRedirect(t *testing.T) {
	var firstQueries atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstQueries.Add(1)
		if r.URL.Path != "/emby/Items" {
			t.Errorf("unexpected first server path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"Items":[]}`))
	}))
	defer first.Close()

	var secondQueries atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emby/Items":
			secondQueries.Add(1)
			if got := r.URL.Query().Get("IncludeItemTypes"); got != "Movie" {
				t.Errorf("unexpected item type %q", got)
			}
			if got := r.URL.Query().Get("AnyProviderIdEquals"); got != "tmdb.278" {
				t.Errorf("unexpected provider query %q", got)
			}
			if got := r.Header.Get("X-Emby-Token"); got != "second-key" {
				t.Errorf("unexpected API token %q", got)
			}
			_, _ = w.Write([]byte(`{"Items":[{"Id":"movie-1","Name":"肖申克的救赎"}]}`))
		case "/emby/Videos/movie-1/stream":
			http.Redirect(w, r, "/media/movie.mkv", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer second.Close()

	client, err := NewClient([]ServerConfig{
		{Name: "首选", URL: first.URL, APIKey: "first-key"},
		{Name: "备用", URL: second.URL, APIKey: "second-key"},
	}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 278})
	if err != nil {
		t.Fatal(err)
	}
	if firstQueries.Load() != 1 || secondQueries.Load() != 1 {
		t.Fatalf("unexpected query counts: first=%d second=%d", firstQueries.Load(), secondQueries.Load())
	}
	if link.ServerName != "备用" || link.ItemName != "肖申克的救赎" || link.PlaybackMode != "stream" {
		t.Fatalf("unexpected link metadata: %#v", link)
	}
	if link.RedirectedURL != second.URL+"/media/movie.mkv" {
		t.Fatalf("unexpected redirect URL %q", link.RedirectedURL)
	}
	if !strings.Contains(link.PlayURL, "/emby/Videos/movie-1/stream") || !strings.Contains(link.PlayURL, "api_key=second-key") {
		t.Fatalf("unexpected play URL %q", link.PlayURL)
	}
}

func TestPlayLinkReturnsSeriesWebPage(t *testing.T) {
	var streamRequested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Items" {
			streamRequested.Store(true)
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("IncludeItemTypes"); got != "Series" {
			t.Errorf("unexpected item type %q", got)
		}
		_, _ = w.Write([]byte(`{"Items":[{"Id":"series-1","Name":"黑镜"}]}`))
	}))
	defer server.Close()

	client, err := NewClient([]ServerConfig{{URL: server.URL, APIKey: "key"}}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tv", ID: 42009})
	if err != nil {
		t.Fatal(err)
	}
	if streamRequested.Load() {
		t.Fatal("series should not request a video stream")
	}
	if link.PlaybackMode != "series" || link.PlayURL != server.URL+"/web/index.html#!/item?id=series-1" {
		t.Fatalf("unexpected series link: %#v", link)
	}
}

func TestPlayLinkReportsMissingAndUnconfigured(t *testing.T) {
	unconfigured, err := NewClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unconfigured.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 1}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected not configured, got %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Items":[]}`))
	}))
	defer server.Close()
	configured, err := NewClient([]ServerConfig{{URL: server.URL, APIKey: "key"}}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configured.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestNewClientRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewClient([]ServerConfig{{URL: "file:///tmp/emby", APIKey: "key"}}, nil); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := NewClient([]ServerConfig{{URL: "https://example.com"}}, nil); err == nil {
		t.Fatal("expected missing key error")
	}
}
