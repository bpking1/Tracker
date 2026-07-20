package metadata

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"traker/internal/domain"
)

func TestFetchCachesMetadataAndPoster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/42":
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Fatal("missing bearer token")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"title":"测试电影","original_title":"Test Film","release_date":"2026-01-02","overview":"简介","poster_path":"/poster.jpg","vote_average":8.2,"credits":{"cast":[{"name":"演员甲"},{"name":"演员乙"}]}}`)
		case "/poster.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xff, 0xd8, 0xff, 0xd9})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dataFile := filepath.Join(t.TempDir(), "traker.txt")
	cache, err := NewCache(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{token: "test-token", apiBase: server.URL, imageBase: server.URL, http: &http.Client{Timeout: time.Second}}
	item, err := client.Fetch(context.Background(), cache, domain.MediaRef{Type: "tm", ID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "测试电影" || item.PosterURL != "/api/images/tm-42.jpg" || len(item.Cast) != 2 {
		t.Fatalf("unexpected metadata: %#v", item)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(dataFile), "cache", "metadata.json"))
	if err != nil || !strings.Contains(string(data), "测试电影") {
		t.Fatalf("metadata was not persisted: %v", err)
	}
	image, contentType, err := cache.OpenImage("tm-42.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	if contentType != "image/jpeg" {
		t.Fatalf("unexpected content type %q", contentType)
	}

	snapshot := domain.Snapshot{Records: []domain.Record{{MediaRef: &domain.MediaRef{Type: "tm", ID: 42}}}}
	cache.Enrich(&snapshot)
	if snapshot.Records[0].Metadata == nil || snapshot.Records[0].Metadata.Title != "测试电影" {
		t.Fatal("snapshot was not enriched from cache")
	}
}

func TestSearchFiltersPeopleFromMultiResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"results":[{"id":1,"media_type":"movie","title":"电影"},{"id":2,"media_type":"person","name":"演员"},{"id":3,"media_type":"tv","name":"剧集"}]}`)
	}))
	defer server.Close()
	client := &Client{apiKey: "test-key", apiBase: server.URL, http: server.Client()}
	results, err := client.Search(context.Background(), "测试", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Type != "movie" || results[1].Type != "tv" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
