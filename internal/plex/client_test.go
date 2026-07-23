package plex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"traker/internal/domain"
	"traker/internal/playback"
)

func TestPlayLinkFindsMovieByTMDBGUID(t *testing.T) {
	var sectionQueries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Plex-Token"); got != "plex-token" {
			t.Errorf("unexpected Plex token %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("unexpected Accept header %q", got)
		}
		switch r.URL.Path {
		case "/library/sections":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"1","type":"movie","title":"电影"},{"key":"2","type":"show","title":"剧集"}]}}`))
		case "/library/sections/1/all":
			sectionQueries.Add(1)
			if got := r.URL.Query().Get("type"); got != "1" {
				t.Errorf("unexpected media type %q", got)
			}
			if got := r.URL.Query().Get("includeGuids"); got != "1" {
				t.Errorf("unexpected includeGuids %q", got)
			}
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":2,"totalSize":2,"Metadata":[
				{"ratingKey":"10","title":"其他影片","Guid":[{"id":"tmdb://1"}]},
				{"ratingKey":"278","title":"肖申克的救赎","Guid":[{"id":"tmdb://278"}],"Media":[{"Part":[{"key":"/library/parts/278/1/movie%20file.mkv"}]}]}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient([]ServerConfig{{Name: "家中 Plex", URL: server.URL, Token: "plex-token"}}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 278})
	if err != nil {
		t.Fatal(err)
	}
	if sectionQueries.Load() != 1 {
		t.Fatalf("unexpected section query count %d", sectionQueries.Load())
	}
	if link.ServerName != "家中 Plex" || link.ItemName != "肖申克的救赎" || link.PlaybackMode != "stream" {
		t.Fatalf("unexpected link metadata: %#v", link)
	}
	parsed, err := url.Parse(link.PlayURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/library/parts/278/1/movie file.mkv" || parsed.Query().Get("X-Plex-Token") != "plex-token" {
		t.Fatalf("unexpected play URL %q", link.PlayURL)
	}
	if link.RedirectedURL != "" {
		t.Fatalf("Plex link should not be probed for redirects: %#v", link)
	}
}

func TestPlayLinkFetchesMetadataDetailWhenPartIsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/sections":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"movies","type":"movie","title":"Movies"}]}}`))
		case "/library/sections/movies/all":
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"Metadata":[{"ratingKey":"42","title":"影片","Guid":[{"id":"themoviedb://42"}]}]}}`))
		case "/library/metadata/42":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"42","title":"影片","Media":[{"Part":[{"key":"/library/parts/42/file.mkv"}]}]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient([]ServerConfig{{URL: server.URL, Token: "token"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(link.PlayURL, "/library/parts/42/file.mkv") {
		t.Fatalf("unexpected play URL %q", link.PlayURL)
	}
}

func TestPlayLinkReportsMissingUnsupportedAndUnconfigured(t *testing.T) {
	unconfigured, err := NewClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unconfigured.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 1}); !errors.Is(err, playback.ErrNotConfigured) {
		t.Fatalf("expected not configured, got %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/sections" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	configured, err := NewClient([]ServerConfig{{URL: server.URL, Token: "token"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configured.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 1}); !errors.Is(err, playback.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := configured.PlayLink(context.Background(), domain.MediaRef{Type: "tv", ID: 1}); !errors.Is(err, playback.ErrUnsupported) {
		t.Fatalf("expected unsupported, got %v", err)
	}
}

func TestNewClientRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewClient([]ServerConfig{{URL: "file:///tmp/plex", Token: "token"}}, nil); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := NewClient([]ServerConfig{{URL: "https://plex.example"}}, nil); err == nil {
		t.Fatal("expected missing token error")
	}
}
