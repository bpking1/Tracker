package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"traker/internal/domain"
	"traker/internal/metadata"
	"traker/internal/store"
)

type Server struct {
	store    *store.Store
	static   fs.FS
	metadata *metadata.Cache
	tmdb     *metadata.Client
}

func New(recordStore *store.Store, static fs.FS) (http.Handler, error) {
	metadataCache, err := metadata.NewCache(recordStore.Path())
	if err != nil {
		return nil, err
	}
	s := &Server{store: recordStore, static: static, metadata: metadataCache, tmdb: metadata.NewClientFromEnvironment()}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/records", s.records)
	mux.HandleFunc("/api/records/", s.recordByKey)
	mux.HandleFunc("/api/tmdb/search", s.tmdbSearch)
	mux.HandleFunc("/api/tmdb/config", s.tmdbConfig)
	mux.HandleFunc("/api/images/", s.image)
	mux.HandleFunc("/api/events", s.events)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if static != nil {
		mux.HandleFunc("/", s.staticFile)
	}
	return logging(mux), nil
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
