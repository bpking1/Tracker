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
	var hubSearchQueries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Plex-Token"); got != "plex-token" {
			t.Errorf("unexpected Plex token %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("unexpected Accept header %q", got)
		}
		switch r.URL.Path {
		case "/hubs/search":
			hubSearchQueries.Add(1)
			if r.URL.Query().Get("query") == "肖申克的救赎" {
				_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[
					{"type":"movie","Metadata":[
						{"ratingKey":"10","title":"其他影片","Guid":[{"id":"tmdb://1"}]},
						{"ratingKey":"278","title":"肖申克的救赎","Guid":[{"id":"tmdb://278"}],"Media":[{"Part":[{"key":"/library/parts/278/1/movie%20file.mkv"}]}]}
					]}
				]}}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient([]ServerConfig{{Name: "家中 Plex", URL: server.URL, Token: "plex-token"}}, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 278, Title: "肖申克的救赎"})
	if err != nil {
		t.Fatal(err)
	}
	if hubSearchQueries.Load() != 1 {
		t.Fatalf("unexpected search query count %d", hubSearchQueries.Load())
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
		case "/hubs/search":
			if r.URL.Query().Get("query") == "影片" {
				_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"type":"movie","Metadata":[{"ratingKey":"42","title":"影片","Guid":[{"id":"themoviedb://42"}]}]}]}}`))
			}
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
	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 42, Title: "影片"})
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
		if r.URL.Path == "/hubs/search" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	configured, err := NewClient([]ServerConfig{{URL: server.URL, Token: "token"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configured.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 1, Title: "不存在"}); !errors.Is(err, playback.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := configured.PlayLink(context.Background(), domain.MediaRef{Type: "tv", ID: 1}); !errors.Is(err, playback.ErrUnsupported) {
		t.Fatalf("expected unsupported, got %v", err)
	}
}

func TestPlayLinkUsesTitleForHubSearch(t *testing.T) {
	var hubSearchCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hubs/search" && r.URL.Query().Get("query") == "Hub 影片" {
			hubSearchCalled = true
			_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[
				{"type":"movie","Metadata":[{"ratingKey":"999","title":"Hub 影片","Guid":[{"id":"tmdb://999"}],"Media":[{"Part":[{"key":"/library/parts/999/file.mkv"}]}]}]}
			]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient([]ServerConfig{{Name: "Plex Hub", URL: server.URL, Token: "token"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 999, Title: "Hub 影片"})
	if err != nil {
		t.Fatal(err)
	}
	if !hubSearchCalled {
		t.Fatal("expected Hub search to be called")
	}
	if link.ItemName != "Hub 影片" || !strings.Contains(link.PlayURL, "/library/parts/999/file.mkv") {
		t.Fatalf("unexpected link result: %#v", link)
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

type fakeMetadataCache struct {
	items map[string]domain.MediaMetadata
}

func (f *fakeMetadataCache) Get(ref domain.MediaRef) (domain.MediaMetadata, bool) {
	m, ok := f.items[ref.Type+":"+strings.TrimSpace(domain.MediaRef{ID: ref.ID}.Title)]
	if !ok {
		m, ok = f.items[ref.Type+":id"]
	}
	return m, ok
}

func TestPlayLinkHubSearchUsesTitleFromMetadata(t *testing.T) {
	var searchedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hubs/search" {
			searchedQuery = r.URL.Query().Get("query")
			if searchedQuery == "肖申克的救赎" {
				_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[
					{"type":"movie","Metadata":[{"ratingKey":"100","title":"肖申克的救赎","Guid":[{"id":"tmdb://278"}],"Media":[{"Part":[{"key":"/library/parts/100/shawshank.mkv"}]}]}]}
				]}}`))
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient([]ServerConfig{{Name: "Plex Hub", URL: server.URL, Token: "token"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	cache := &fakeMetadataCache{
		items: map[string]domain.MediaMetadata{
			"tm:id": {
				MediaRef:      domain.MediaRef{Type: "tm", ID: 278},
				Title:         "肖申克的救赎",
				OriginalTitle: "The Shawshank Redemption",
			},
		},
	}
	client.SetMetadataCache(cache)

	link, err := client.PlayLink(context.Background(), domain.MediaRef{Type: "tm", ID: 278})
	if err != nil {
		t.Fatal(err)
	}
	if searchedQuery != "肖申克的救赎" {
		t.Fatalf("expected search query to be '肖申克的救赎', got %q", searchedQuery)
	}
	if link.ItemName != "肖申克的救赎" || !strings.Contains(link.PlayURL, "/library/parts/100/shawshank.mkv") {
		t.Fatalf("unexpected play link: %#v", link)
	}
}

func TestSeriesCatalogSearchesByNameAndVerifiesTMDBGUID(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hubs/search":
			queries = append(queries, r.URL.Query().Get("query"))
			_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[
				{"type":"movie","Metadata":[{"ratingKey":"8","type":"movie","title":"同名电影","Guid":[{"id":"tmdb://1396"}]}]},
				{"type":"show","Metadata":[
					{"ratingKey":"10","type":"show","title":"同名错误剧集","Guid":[{"id":"tmdb://1"}]},
					{"ratingKey":"20","type":"show","title":"绝命毒师"}
				]}
			]}}`))
		case "/library/metadata/20":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"20","type":"show","title":"绝命毒师","Guid":[{"id":"tmdb://1396"}]}]}}`))
		case "/library/metadata/20/children":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
				{"ratingKey":"200","type":"season","index":0,"title":"特别篇"},
				{"ratingKey":"201","type":"season","index":1,"title":"第 1 季"}
			]}}`))
		case "/library/metadata/200/children":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
				{"ratingKey":"2001","type":"episode","parentIndex":0,"index":1,"title":"花絮","duration":600000,"originallyAvailableAt":"2008-01-01"}
			]}}`))
		case "/library/metadata/201/children":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
				{"ratingKey":"2011","type":"episode","parentIndex":1,"index":1,"title":"试播集","duration":3480000,"originallyAvailableAt":"2008-01-20"}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient([]ServerConfig{{Name: "剧集 Plex", URL: server.URL, Token: "series-token"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.SeriesCatalog(context.Background(), domain.MediaRef{Type: "tv", ID: 1396, Title: "绝命毒师"})
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0] != "绝命毒师" {
		t.Fatalf("series search must use its name, got %#v", queries)
	}
	if catalog.SeriesKey != "20" || catalog.ServerID != "0" || catalog.ServerName != "剧集 Plex" || len(catalog.Seasons) != 2 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	if catalog.Seasons[0].Number != 0 || catalog.Seasons[0].Episodes[0].SeasonNumber != 0 {
		t.Fatalf("special season was not preserved: %#v", catalog.Seasons[0])
	}
	episode := catalog.Seasons[1].Episodes[0]
	if episode.RatingKey != "2011" || episode.SeasonNumber != 1 || episode.EpisodeNumber != 1 || episode.Duration != 3480000 {
		t.Fatalf("unexpected episode: %#v", episode)
	}
}

func TestEpisodePlayLinkValidatesSeriesAndReturnsPart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/2011" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{
			"ratingKey":"2011","grandparentRatingKey":"20","type":"episode","title":"试播集",
			"Media":[{"Part":[{"key":"/library/parts/2011/episode%201.mkv"}]}]
		}]}}`))
	}))
	defer server.Close()
	client, err := NewClient([]ServerConfig{{Name: "剧集 Plex", URL: server.URL, Token: "series-token"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	link, err := client.EpisodePlayLink(context.Background(), "0", "20", "2011")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(link.PlayURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/library/parts/2011/episode 1.mkv" || parsed.Query().Get("X-Plex-Token") != "series-token" || link.ItemName != "试播集" {
		t.Fatalf("unexpected episode link: %#v", link)
	}
	if _, err := client.EpisodePlayLink(context.Background(), "0", "21", "2011"); !errors.Is(err, playback.ErrNotFound) {
		t.Fatalf("expected mismatched series to be rejected, got %v", err)
	}
	if _, err := client.EpisodePlayLink(context.Background(), "bad", "20", "2011"); !errors.Is(err, ErrInvalidSeriesRequest) {
		t.Fatalf("expected invalid request, got %v", err)
	}
}
