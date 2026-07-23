package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"traker/internal/domain"
	"traker/internal/emby"
	"traker/internal/metadata"
	"traker/internal/store"
)

type fakeMetadataClient struct{}

func (fakeMetadataClient) Configured() bool { return true }

func (fakeMetadataClient) Search(_ context.Context, query, _ string) ([]metadata.SearchResult, error) {
	if query == "没有结果" {
		return []metadata.SearchResult{}, nil
	}
	return []metadata.SearchResult{{ID: 42, Type: "movie", Title: "第一条结果"}}, nil
}

func (fakeMetadataClient) Fetch(_ context.Context, cache *metadata.Cache, mediaRef domain.MediaRef) (domain.MediaMetadata, error) {
	item := domain.MediaMetadata{MediaRef: mediaRef, Title: "第一条结果", Genres: []string{}}
	return item, cache.Put(item)
}

type fakePlaybackClient struct {
	configured bool
	link       emby.PlayLink
	err        error
	received   domain.MediaRef
}

func (client *fakePlaybackClient) Configured() bool { return client.configured }
func (client *fakePlaybackClient) PlayLink(_ context.Context, mediaRef domain.MediaRef) (emby.PlayLink, error) {
	client.received = mediaRef
	return client.link, client.err
}

func TestAutoMatchUsesFirstResultAndWritesOnce(t *testing.T) {
	dataFile := filepath.Join(t.TempDir(), "traker.txt")
	if err := os.WriteFile(dataFile, []byte("- 自动匹配\n- 没有结果\nx 已有 ID tm:7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordStore, err := store.New(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := metadata.NewCache(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(recordStore, nil, cache, fakeMetadataClient{}, nil)
	snapshot, err := recordStore.Read()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"revision": snapshot.Revision})
	request := httptest.NewRequest(http.MethodPost, "/api/tmdb/auto-match", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var result autoMatchResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Matched != 1 || len(result.NoResults) != 1 || len(result.Failed) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "- 自动匹配 tm:42") || !strings.Contains(string(data), "- 没有结果") {
		t.Fatalf("unexpected data file:\n%s", data)
	}
	backups, err := os.ReadDir(filepath.Join(filepath.Dir(dataFile), "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one backup, got %d", len(backups))
	}
}

func TestConfigReturnsAbsoluteDataPath(t *testing.T) {
	dataFile := filepath.Join(t.TempDir(), "traker.txt")
	recordStore, err := store.New(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := metadata.NewCache(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(recordStore, nil, cache, fakeMetadataClient{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected status %d: %s", response.Code, body)
	}
	var config map[string]string
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
		t.Fatal(err)
	}
	if config["dataFile"] != recordStore.Path() || !filepath.IsAbs(config["dataFile"]) {
		t.Fatalf("unexpected data path %q", config["dataFile"])
	}
}

func TestRefreshMissingMetadataDoesNotRewriteDataFile(t *testing.T) {
	dataFile := filepath.Join(t.TempDir(), "traker.txt")
	original := "- 已匹配 tm:42\n- 未匹配\n"
	if err := os.WriteFile(dataFile, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	recordStore, err := store.New(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := metadata.NewCache(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(recordStore, nil, cache, fakeMetadataClient{}, nil)
	snapshot, err := recordStore.Read()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"revision": snapshot.Revision})
	request := httptest.NewRequest(http.MethodPost, "/api/tmdb/refresh-missing", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var result refreshMetadataResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Refreshed != 1 || len(result.Failed) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Snapshot.Records[0].Metadata == nil || result.Snapshot.Records[0].MetadataState != "ready" {
		t.Fatalf("metadata was not refreshed: %#v", result.Snapshot.Records[0])
	}
	data, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("data file was rewritten:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dataFile), "backups")); !os.IsNotExist(err) {
		t.Fatalf("metadata refresh should not create text backups: %v", err)
	}
}

func TestPlayLinkReturnsConfiguredEmbyResult(t *testing.T) {
	dataFile := filepath.Join(t.TempDir(), "traker.txt")
	recordStore, err := store.New(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := metadata.NewCache(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	playback := &fakePlaybackClient{
		configured: true,
		link:       emby.PlayLink{PlayURL: "https://emby.example/stream", ItemName: "测试影片", PlaybackMode: "stream"},
	}
	handler := newHandler(recordStore, nil, cache, fakeMetadataClient{}, playback)
	request := httptest.NewRequest(http.MethodGet, "/api/play-link?type=tm&q=278", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	if playback.received != (domain.MediaRef{Type: "tm", ID: 278}) {
		t.Fatalf("unexpected media reference: %#v", playback.received)
	}
	var link emby.PlayLink
	if err := json.NewDecoder(response.Body).Decode(&link); err != nil {
		t.Fatal(err)
	}
	if link.PlayURL != playback.link.PlayURL || link.ItemName != playback.link.ItemName {
		t.Fatalf("unexpected response: %#v", link)
	}
}

func TestPlayLinkStatusCodes(t *testing.T) {
	dataFile := filepath.Join(t.TempDir(), "traker.txt")
	recordStore, err := store.New(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := metadata.NewCache(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		client     playbackClient
		path       string
		wantStatus int
	}{
		{name: "unconfigured", client: nil, path: "/api/play-link?type=tm&q=1", wantStatus: http.StatusServiceUnavailable},
		{name: "invalid query", client: &fakePlaybackClient{configured: true}, path: "/api/play-link?type=bad&q=0", wantStatus: http.StatusBadRequest},
		{name: "not found", client: &fakePlaybackClient{configured: true, err: emby.ErrNotFound}, path: "/api/play-link?type=tv&q=2", wantStatus: http.StatusNotFound},
		{name: "upstream failure", client: &fakePlaybackClient{configured: true, err: errors.New("upstream")}, path: "/api/play-link?type=tm&q=3", wantStatus: http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newHandler(recordStore, nil, cache, fakeMetadataClient{}, test.client)
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
