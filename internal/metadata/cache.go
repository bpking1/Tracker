package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"traker/internal/domain"
)

var ErrNotConfigured = errors.New("TMDB_API_TOKEN or TMDB_API_KEY is not configured")

type Cache struct {
	directory string
	file      string
	images    string
	mu        sync.RWMutex
	items     map[string]domain.MediaMetadata
}

type Client struct {
	token     string
	apiKey    string
	apiBase   string
	imageBase string
	http      *http.Client
}

type SearchResult struct {
	ID          int     `json:"id"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Date        string  `json:"date"`
	Overview    string  `json:"overview"`
	PosterPath  string  `json:"posterPath"`
	VoteAverage float64 `json:"voteAverage"`
}

func NewCache(dataFile string) (*Cache, error) {
	directory := filepath.Join(filepath.Dir(dataFile), "cache")
	images := filepath.Join(directory, "images")
	if err := os.MkdirAll(images, 0o755); err != nil {
		return nil, err
	}
	cache := &Cache{directory: directory, file: filepath.Join(directory, "metadata.json"), images: images, items: map[string]domain.MediaMetadata{}}
	data, err := os.ReadFile(cache.file)
	if errors.Is(err, os.ErrNotExist) {
		return cache, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cache.items); err != nil {
			corruptName := cache.file + ".corrupt-" + time.Now().Format("20060102-150405")
			_ = os.Rename(cache.file, corruptName)
			cache.items = map[string]domain.MediaMetadata{}
		}
	}
	return cache, nil
}

func NewClientFromEnvironment() *Client {
	return &Client{
		token:     strings.TrimSpace(os.Getenv("TMDB_API_TOKEN")),
		apiKey:    strings.TrimSpace(os.Getenv("TMDB_API_KEY")),
		apiBase:   "https://api.themoviedb.org/3",
		imageBase: "https://image.tmdb.org/t/p/w342",
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Configured() bool { return c.token != "" || c.apiKey != "" }

func (c *Cache) Enrich(snapshot *domain.Snapshot) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for index := range snapshot.Records {
		snapshot.Records[index].Metadata = nil
		mediaRef := snapshot.Records[index].MediaRef
		if mediaRef == nil {
			snapshot.Records[index].MetadataState = "unmatched"
			continue
		}
		snapshot.Records[index].MetadataState = "missing"
		if item, ok := c.items[cacheKey(*mediaRef)]; ok {
			copy := item
			snapshot.Records[index].MetadataState = "ready"
			if copy.Genres == nil {
				snapshot.Records[index].MetadataState = "invalid"
				copy.Genres = []string{}
			}
			if copy.PosterURL != "" {
				cacheName := filepath.Base(strings.TrimPrefix(copy.PosterURL, "/api/images/"))
				if _, err := os.Stat(filepath.Join(c.images, cacheName)); err != nil {
					copy.PosterURL = ""
					snapshot.Records[index].MetadataState = "invalid"
				}
			}
			snapshot.Records[index].Metadata = &copy
		}
	}
}

func (c *Cache) Get(mediaRef domain.MediaRef) (domain.MediaMetadata, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[cacheKey(mediaRef)]
	return item, ok
}

func (c *Cache) Put(item domain.MediaMetadata) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[cacheKey(item.MediaRef)] = item
	data, err := json.MarshalIndent(c.items, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(c.directory, ".metadata-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err = temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Remove(c.file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tempPath, c.file)
}

func (c *Cache) SaveImage(cacheName string, content io.Reader) error {
	if filepath.Base(cacheName) != cacheName || cacheName == "." || cacheName == "" {
		return fmt.Errorf("invalid image cache key")
	}
	temp, err := os.CreateTemp(c.images, ".image-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err = io.Copy(temp, io.LimitReader(content, 15<<20)); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	destination := filepath.Join(c.images, cacheName)
	_ = os.Remove(destination)
	return os.Rename(tempPath, destination)
}

func (c *Cache) OpenImage(cacheName string) (*os.File, string, error) {
	if filepath.Base(cacheName) != cacheName || cacheName == "." || cacheName == "" {
		return nil, "", os.ErrNotExist
	}
	file, err := os.Open(filepath.Join(c.images, cacheName))
	if err != nil {
		return nil, "", err
	}
	contentType := mime.TypeByExtension(filepath.Ext(cacheName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return file, contentType, nil
}

func (c *Client) Search(ctx context.Context, query, typeFilter string) ([]SearchResult, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	if typeFilter != "movie" && typeFilter != "tv" {
		typeFilter = "multi"
	}
	var payload struct {
		Results []struct {
			ID           int     `json:"id"`
			MediaType    string  `json:"media_type"`
			Title        string  `json:"title"`
			Name         string  `json:"name"`
			ReleaseDate  string  `json:"release_date"`
			FirstAirDate string  `json:"first_air_date"`
			Overview     string  `json:"overview"`
			PosterPath   string  `json:"poster_path"`
			VoteAverage  float64 `json:"vote_average"`
		} `json:"results"`
	}
	if err := c.getJSON(ctx, "/search/"+typeFilter, map[string]string{"query": query, "language": "zh-CN"}, &payload); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		kind, title, date := item.MediaType, item.Title, item.ReleaseDate
		if kind == "" {
			kind = typeFilter
		}
		if kind == "tv" {
			title, date = item.Name, item.FirstAirDate
		}
		if kind != "movie" && kind != "tv" {
			continue
		}
		results = append(results, SearchResult{ID: item.ID, Type: kind, Title: title, Date: date, Overview: item.Overview, PosterPath: item.PosterPath, VoteAverage: item.VoteAverage})
	}
	return results, nil
}

func (c *Client) Fetch(ctx context.Context, cache *Cache, mediaRef domain.MediaRef) (domain.MediaMetadata, error) {
	if !c.Configured() {
		return domain.MediaMetadata{}, ErrNotConfigured
	}
	kind := "movie"
	if mediaRef.Type == "tv" {
		kind = "tv"
	}
	var payload struct {
		Title         string  `json:"title"`
		Name          string  `json:"name"`
		OriginalTitle string  `json:"original_title"`
		OriginalName  string  `json:"original_name"`
		ReleaseDate   string  `json:"release_date"`
		FirstAirDate  string  `json:"first_air_date"`
		Overview      string  `json:"overview"`
		PosterPath    string  `json:"poster_path"`
		VoteAverage   float64 `json:"vote_average"`
		Genres        []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Credits struct {
			Cast []struct {
				Name string `json:"name"`
			} `json:"cast"`
		} `json:"credits"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("/%s/%d", kind, mediaRef.ID), map[string]string{"language": "zh-CN", "append_to_response": "credits"}, &payload); err != nil {
		return domain.MediaMetadata{}, err
	}
	title, originalTitle, releaseDate := payload.Title, payload.OriginalTitle, payload.ReleaseDate
	if mediaRef.Type == "tv" {
		title, originalTitle, releaseDate = payload.Name, payload.OriginalName, payload.FirstAirDate
	}
	cast := make([]string, 0, min(8, len(payload.Credits.Cast)))
	for _, person := range payload.Credits.Cast {
		if person.Name != "" {
			cast = append(cast, person.Name)
		}
		if len(cast) == 8 {
			break
		}
	}
	genres := make([]string, 0, len(payload.Genres))
	for _, genre := range payload.Genres {
		if genre.Name != "" {
			genres = append(genres, genre.Name)
		}
	}
	item := domain.MediaMetadata{MediaRef: mediaRef, Title: title, OriginalTitle: originalTitle, ReleaseDate: releaseDate, Overview: payload.Overview, Genres: genres, Cast: cast, VoteAverage: payload.VoteAverage, FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	if payload.PosterPath != "" {
		cacheName := fmt.Sprintf("%s-%d%s", mediaRef.Type, mediaRef.ID, imageExtension(payload.PosterPath))
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.imageBase+payload.PosterPath, nil)
		if err == nil {
			response, requestErr := c.http.Do(request)
			if requestErr == nil {
				defer response.Body.Close()
				if response.StatusCode == http.StatusOK && cache.SaveImage(cacheName, response.Body) == nil {
					item.PosterURL = "/api/images/" + cacheName
				}
			}
		}
	}
	if err := cache.Put(item); err != nil {
		return domain.MediaMetadata{}, err
	}
	return item, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, params map[string]string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+endpoint, nil)
	if err != nil {
		return err
	}
	query := request.URL.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	if c.apiKey != "" && c.token == "" {
		query.Set("api_key", c.apiKey)
	}
	request.URL.RawQuery = query.Encode()
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("TMDB request failed (%d): %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func cacheKey(mediaRef domain.MediaRef) string {
	return fmt.Sprintf("%s:%d", mediaRef.Type, mediaRef.ID)
}

func imageExtension(posterPath string) string {
	extension := strings.ToLower(filepath.Ext(posterPath))
	if extension == ".jpg" || extension == ".jpeg" || extension == ".png" || extension == ".webp" {
		return extension
	}
	return ".jpg"
}
