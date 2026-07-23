package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"traker/internal/domain"
	"traker/internal/emby"
	"traker/internal/metadata"
	"traker/internal/playback"
	"traker/internal/plex"
	"traker/internal/store"
)

type Server struct {
	store    *store.Store
	static   fs.FS
	metadata *metadata.Cache
	tmdb     metadataClient
	playback playbackClient
}

type metadataClient interface {
	Configured() bool
	Search(context.Context, string, string) ([]metadata.SearchResult, error)
	Fetch(context.Context, *metadata.Cache, domain.MediaRef) (domain.MediaMetadata, error)
}

type playbackClient interface {
	Configured() bool
	PlayLink(context.Context, domain.MediaRef) (playback.Link, error)
}

func New(recordStore *store.Store, static fs.FS) (http.Handler, error) {
	metadataCache, err := metadata.NewCache(recordStore.Path())
	if err != nil {
		return nil, err
	}
	embyClient, err := emby.NewClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	plexClient, err := plex.NewClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	playbackClient := playback.NewClient(embyClient, plexClient)
	return newHandler(recordStore, static, metadataCache, metadata.NewClientFromEnvironment(), playbackClient), nil
}

func newHandler(recordStore *store.Store, static fs.FS, metadataCache *metadata.Cache, tmdb metadataClient, playbackClient playbackClient) http.Handler {
	s := &Server{store: recordStore, static: static, metadata: metadataCache, tmdb: tmdb, playback: playbackClient}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/records", s.records)
	mux.HandleFunc("/api/records/", s.recordByKey)
	mux.HandleFunc("/api/tmdb/search", s.tmdbSearch)
	mux.HandleFunc("/api/tmdb/config", s.tmdbConfig)
	mux.HandleFunc("/api/tmdb/auto-match", s.tmdbAutoMatch)
	mux.HandleFunc("/api/tmdb/refresh-missing", s.tmdbRefreshMissing)
	mux.HandleFunc("/api/play-link", s.playLink)
	mux.HandleFunc("/api/config", s.config)
	mux.HandleFunc("/api/images/", s.image)
	mux.HandleFunc("/api/events", s.events)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if static != nil {
		mux.HandleFunc("/", s.staticFile)
	}
	return logging(mux)
}

func (s *Server) records(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		snapshot, err := s.store.Read()
		if err != nil {
			errorJSON(w, err)
			return
		}
		s.metadata.Enrich(&snapshot)
		writeJSON(w, http.StatusOK, snapshot)
	case http.MethodPost:
		var request revisionInput
		if !decode(w, r, &request) {
			return
		}
		snapshot, err := s.store.Add(request.Revision, request.RecordInput)
		s.writeMutation(w, snapshot, err)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) recordByKey(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/records/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "tmdb-match" && r.Method == http.MethodPost {
		var request matchInput
		if !decode(w, r, &request) {
			return
		}
		s.matchTmdb(w, r, parts[0], request)
		return
	}
	if len(parts) == 2 && parts[1] == "tmdb-refresh" && r.Method == http.MethodPost {
		var request revisionInput
		if !decode(w, r, &request) {
			return
		}
		s.refreshTmdb(w, r, parts[0], request.Revision)
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	key := parts[0]
	var request revisionInput
	if r.Method == http.MethodPut {
		if !decode(w, r, &request) {
			return
		}
		snapshot, err := s.store.Update(request.Revision, key, request.RecordInput)
		s.writeMutation(w, snapshot, err)
		return
	}
	if r.Method == http.MethodDelete {
		if !decode(w, r, &request) {
			return
		}
		snapshot, err := s.store.Delete(request.Revision, key)
		s.writeMutation(w, snapshot, err)
		return
	}
	methodNotAllowed(w)
}

type revisionInput struct {
	domain.RecordInput
	Revision string `json:"revision"`
}
type matchInput struct {
	Revision string `json:"revision"`
	Type     string `json:"type"`
	ID       int    `json:"id"`
}

func (s *Server) writeMutation(w http.ResponseWriter, snapshot domain.Snapshot, err error) {
	if errors.Is(err, store.ErrConflict) {
		writeJSON(w, http.StatusConflict, snapshot)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		errorJSON(w, err)
		return
	}
	s.metadata.Enrich(&snapshot)
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) tmdbSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	results, err := s.tmdb.Search(r.Context(), query, r.URL.Query().Get("type"))
	if err != nil {
		s.tmdbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) tmdbConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"configured": s.tmdb.Configured()})
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"dataFile": s.store.Path()})
}

func (s *Server) playLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.playback == nil || !s.playback.Configured() {
		errorJSONStatus(w, http.StatusServiceUnavailable, playback.ErrNotConfigured)
		return
	}
	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	if mediaType == "" {
		mediaType = "tm"
	}
	id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil || id <= 0 || (mediaType != "tm" && mediaType != "tv") {
		errorJSONStatus(w, http.StatusBadRequest, errors.New("需要有效的 TMDB ID 和类型"))
		return
	}
	link, err := s.playback.PlayLink(r.Context(), domain.MediaRef{Type: mediaType, ID: id})
	if errors.Is(err, playback.ErrNotConfigured) {
		errorJSONStatus(w, http.StatusServiceUnavailable, err)
		return
	}
	if errors.Is(err, playback.ErrNotFound) {
		errorJSONStatus(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, playback.ErrUnsupported) {
		errorJSONStatus(w, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		errorJSONStatus(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

type autoMatchFailure struct {
	Title string `json:"title"`
	Error string `json:"error"`
}

type autoMatchResult struct {
	Snapshot  domain.Snapshot    `json:"snapshot"`
	Total     int                `json:"total"`
	Matched   int                `json:"matched"`
	NoResults []string           `json:"noResults"`
	Failed    []autoMatchFailure `json:"failed"`
}

type refreshMetadataResult struct {
	Snapshot  domain.Snapshot    `json:"snapshot"`
	Total     int                `json:"total"`
	Refreshed int                `json:"refreshed"`
	Failed    []autoMatchFailure `json:"failed"`
}

func (s *Server) tmdbAutoMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var request revisionInput
	if !decode(w, r, &request) {
		return
	}
	if !s.tmdb.Configured() {
		s.tmdbError(w, metadata.ErrNotConfigured)
		return
	}
	snapshot, err := s.store.Read()
	if err != nil {
		errorJSON(w, err)
		return
	}
	if snapshot.Revision != request.Revision {
		writeJSON(w, http.StatusConflict, snapshot)
		return
	}

	result := autoMatchResult{Snapshot: snapshot, NoResults: []string{}, Failed: []autoMatchFailure{}}
	matches := make([]store.MatchInput, 0)
	for _, record := range snapshot.Records {
		if record.MediaRef != nil {
			continue
		}
		result.Total++
		if len(record.Warnings) > 0 {
			result.Failed = append(result.Failed, autoMatchFailure{Title: record.Title, Error: "记录格式有警告"})
			continue
		}
		results, searchErr := s.tmdb.Search(r.Context(), record.Title, "all")
		if searchErr != nil {
			result.Failed = append(result.Failed, autoMatchFailure{Title: record.Title, Error: searchErr.Error()})
			continue
		}
		if len(results) == 0 {
			result.NoResults = append(result.NoResults, record.Title)
			continue
		}
		first := results[0]
		mediaType := "tm"
		if first.Type == "tv" {
			mediaType = "tv"
		}
		mediaRef := domain.MediaRef{Type: mediaType, ID: first.ID}
		if _, fetchErr := s.tmdb.Fetch(r.Context(), s.metadata, mediaRef); fetchErr != nil {
			result.Failed = append(result.Failed, autoMatchFailure{Title: record.Title, Error: fetchErr.Error()})
			continue
		}
		matches = append(matches, store.MatchInput{Key: record.Key, Type: mediaType, ID: first.ID})
	}

	if len(matches) > 0 {
		snapshot, err = s.store.BatchMatch(request.Revision, matches)
		if errors.Is(err, store.ErrConflict) {
			writeJSON(w, http.StatusConflict, snapshot)
			return
		}
		if err != nil {
			errorJSON(w, err)
			return
		}
	} else {
		snapshot, err = s.store.Read()
		if err != nil {
			errorJSON(w, err)
			return
		}
		if snapshot.Revision != request.Revision {
			writeJSON(w, http.StatusConflict, snapshot)
			return
		}
	}
	s.metadata.Enrich(&snapshot)
	result.Snapshot = snapshot
	result.Matched = len(matches)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) tmdbRefreshMissing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var request revisionInput
	if !decode(w, r, &request) {
		return
	}
	if !s.tmdb.Configured() {
		s.tmdbError(w, metadata.ErrNotConfigured)
		return
	}
	snapshot, err := s.store.Read()
	if err != nil {
		errorJSON(w, err)
		return
	}
	if snapshot.Revision != request.Revision {
		writeJSON(w, http.StatusConflict, snapshot)
		return
	}
	s.metadata.Enrich(&snapshot)

	result := refreshMetadataResult{Snapshot: snapshot, Failed: []autoMatchFailure{}}
	for _, record := range snapshot.Records {
		if record.MediaRef == nil || (record.MetadataState != "missing" && record.MetadataState != "invalid") {
			continue
		}
		result.Total++
		if _, fetchErr := s.tmdb.Fetch(r.Context(), s.metadata, *record.MediaRef); fetchErr != nil {
			result.Failed = append(result.Failed, autoMatchFailure{Title: record.Title, Error: fetchErr.Error()})
			continue
		}
		result.Refreshed++
	}

	latest, err := s.store.Read()
	if err != nil {
		errorJSON(w, err)
		return
	}
	if latest.Revision != request.Revision {
		writeJSON(w, http.StatusConflict, latest)
		return
	}
	s.metadata.Enrich(&latest)
	result.Snapshot = latest
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) matchTmdb(w http.ResponseWriter, r *http.Request, key string, request matchInput) {
	if (request.Type != "tm" && request.Type != "tv") || request.ID <= 0 {
		errorJSONStatus(w, http.StatusBadRequest, errors.New("invalid media reference"))
		return
	}
	mediaRef := domain.MediaRef{Type: request.Type, ID: request.ID}
	if _, err := s.tmdb.Fetch(r.Context(), s.metadata, mediaRef); err != nil {
		s.tmdbError(w, err)
		return
	}
	snapshot, err := s.store.Match(request.Revision, key, request.Type, request.ID)
	s.writeMutation(w, snapshot, err)
}

func (s *Server) refreshTmdb(w http.ResponseWriter, r *http.Request, key, revision string) {
	snapshot, err := s.store.Read()
	if err != nil {
		errorJSON(w, err)
		return
	}
	if snapshot.Revision != revision {
		writeJSON(w, http.StatusConflict, snapshot)
		return
	}
	var mediaRef *domain.MediaRef
	for _, record := range snapshot.Records {
		if record.Key == key {
			mediaRef = record.MediaRef
			break
		}
	}
	if mediaRef == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "record or TMDB ID not found"})
		return
	}
	if _, err := s.tmdb.Fetch(r.Context(), s.metadata, *mediaRef); err != nil {
		s.tmdbError(w, err)
		return
	}
	s.metadata.Enrich(&snapshot)
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	cacheName := strings.TrimPrefix(r.URL.Path, "/api/images/")
	file, contentType, err := s.metadata.OpenImage(cacheName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, cacheName, info.ModTime(), file)
}

func (s *Server) tmdbError(w http.ResponseWriter, err error) {
	if errors.Is(err, metadata.ErrNotConfigured) {
		errorJSONStatus(w, http.StatusServiceUnavailable, err)
		return
	}
	errorJSONStatus(w, http.StatusBadGateway, err)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	initial, err := s.store.Read()
	if err != nil {
		errorJSON(w, err)
		return
	}
	fmt.Fprintf(w, "event: ready\ndata: %s\n\n", mustJSON(map[string]string{"revision": initial.Revision}))
	flusher.Flush()
	ticker := time.NewTicker(900 * time.Millisecond)
	defer ticker.Stop()
	previous := initial.Revision
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snapshot, err := s.store.Read()
			if err != nil || snapshot.Revision == previous {
				continue
			}
			previous = snapshot.Revision
			fmt.Fprintf(w, "event: changed\ndata: %s\n\n", mustJSON(map[string]string{"revision": previous}))
			flusher.Flush()
		}
	}
}

func (s *Server) staticFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	data, err := fs.ReadFile(s.static, name)
	if err != nil {
		name = "index.html"
		data, err = fs.ReadFile(s.static, name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		errorJSONStatus(w, http.StatusBadRequest, err)
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func mustJSON(value any) string                  { data, _ := json.Marshal(value); return string(data) }
func errorJSON(w http.ResponseWriter, err error) { errorJSONStatus(w, http.StatusBadRequest, err) }
func errorJSONStatus(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST, PUT, DELETE")
	w.WriteHeader(http.StatusMethodNotAllowed)
}
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started))
	})
}
