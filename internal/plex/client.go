package plex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"traker/internal/domain"
	"traker/internal/playback"
)

var ErrInvalidSeriesRequest = errors.New("无效的 Plex 剧集播放参数")

const childrenPageSize = 200

type ServerConfig struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

type metadataGetter interface {
	Get(domain.MediaRef) (domain.MediaMetadata, bool)
}

type Client struct {
	servers   []server
	http      *http.Client
	metaCache metadataGetter
}

type SeriesCatalog struct {
	SeriesTitle string   `json:"seriesTitle"`
	ServerID    string   `json:"serverId"`
	ServerName  string   `json:"serverName"`
	SeriesKey   string   `json:"seriesKey"`
	Seasons     []Season `json:"seasons"`
}

type Season struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	Episodes []Episode `json:"episodes"`
}

type Episode struct {
	RatingKey    string `json:"ratingKey"`
	SeasonNumber int    `json:"seasonNumber"`
	EpisodeNumber int   `json:"episodeNumber"`
	Title        string `json:"title"`
	Duration     int64  `json:"duration"`
	AirDate      string `json:"airDate"`
}

func (c *Client) SetMetadataCache(cache metadataGetter) {
	if c != nil {
		c.metaCache = cache
	}
}

type server struct {
	name   string
	apiURL *url.URL
	token  string
}

type mediaContainerResponse struct {
	MediaContainer struct {
		Size      int             `json:"size"`
		TotalSize int             `json:"totalSize"`
		Offset    int             `json:"offset"`
		Directory []directory     `json:"Directory"`
		Metadata  []mediaMetadata `json:"Metadata"`
		Hub       []hub           `json:"Hub"`
	} `json:"MediaContainer"`
}

type hub struct {
	Type     string          `json:"type"`
	Metadata []mediaMetadata `json:"Metadata"`
}

type directory struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type mediaMetadata struct {
	RatingKey            string `json:"ratingKey"`
	ParentRatingKey      string `json:"parentRatingKey"`
	GrandparentRatingKey string `json:"grandparentRatingKey"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	GUID                 string `json:"guid"`
	Index                int    `json:"index"`
	ParentIndex          int    `json:"parentIndex"`
	Duration             int64  `json:"duration"`
	OriginallyAvailableAt string `json:"originallyAvailableAt"`
	GUIDs     []struct {
		ID string `json:"id"`
	} `json:"Guid"`
	Media []struct {
		Parts []struct {
			Key string `json:"key"`
		} `json:"Part"`
	} `json:"Media"`
}

func NewClientFromEnvironment() (*Client, error) {
	raw := strings.TrimSpace(os.Getenv("PLEX_SERVERS"))
	if raw == "" {
		return &Client{http: defaultHTTPClient()}, nil
	}
	var configs []ServerConfig
	if err := json.Unmarshal([]byte(raw), &configs); err != nil {
		return nil, fmt.Errorf("PLEX_SERVERS 必须是 JSON 数组: %w", err)
	}
	return NewClient(configs, nil)
}

func NewClient(configs []ServerConfig, httpClient *http.Client) (*Client, error) {
	servers := make([]server, 0, len(configs))
	for index, config := range configs {
		apiURL, err := parseServerURL(config.URL)
		if err != nil {
			return nil, fmt.Errorf("Plex 服务 %d 地址无效: %w", index+1, err)
		}
		token := strings.TrimSpace(config.Token)
		if token == "" {
			return nil, fmt.Errorf("Plex 服务 %d 缺少 Token", index+1)
		}
		name := strings.TrimSpace(config.Name)
		if name == "" {
			name = fmt.Sprintf("Plex %d", index+1)
		}
		servers = append(servers, server{name: name, apiURL: apiURL, token: token})
	}
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	return &Client{servers: servers, http: httpClient}, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

func parseServerURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("仅支持 http 或 https")
	}
	if parsed.Host == "" {
		return nil, errors.New("缺少主机名")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("地址不能包含查询参数或片段")
	}
	return parsed, nil
}

func (c *Client) Configured() bool {
	return c != nil && len(c.servers) > 0
}

func (c *Client) buildSearchQueries(mediaRef domain.MediaRef) []string {
	var queries []string
	addQuery := func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			return
		}
		for _, existing := range queries {
			if existing == q {
				return
			}
		}
		queries = append(queries, q)
	}

	if mediaRef.Title != "" {
		addQuery(mediaRef.Title)
	}
	if c.metaCache != nil {
		if meta, ok := c.metaCache.Get(mediaRef); ok {
			addQuery(meta.Title)
			addQuery(meta.OriginalTitle)
		}
	}
	return queries
}

func (c *Client) PlayLink(ctx context.Context, mediaRef domain.MediaRef) (playback.Link, error) {
	if !c.Configured() {
		return playback.Link{}, playback.ErrNotConfigured
	}
	if (mediaRef.Type != "tm" && mediaRef.Type != "tv") || mediaRef.ID <= 0 {
		return playback.Link{}, errors.New("无效的 TMDB ID")
	}
	if mediaRef.Type == "tv" {
		return playback.Link{}, playback.ErrUnsupported
	}

	queries := c.buildSearchQueries(mediaRef)
	failures := make([]string, 0)
	for _, configuredServer := range c.servers {
		item, err := c.findMovie(ctx, configuredServer, mediaRef.ID, queries)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s 查询失败: %v", configuredServer.name, err))
			continue
		}
		if item == nil {
			continue
		}
		partKey := firstPartKey(*item)
		if partKey == "" && item.RatingKey != "" {
			detail, err := c.metadata(ctx, configuredServer, item.RatingKey)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s 读取媒体信息失败: %v", configuredServer.name, err))
				continue
			}
			if detail != nil {
				item = detail
				partKey = firstPartKey(*item)
			}
		}
		if partKey == "" {
			failures = append(failures, fmt.Sprintf("%s 未返回可播放的媒体 Part", configuredServer.name))
			continue
		}
		playURL := configuredServer.resolve(partKey)
		query := playURL.Query()
		query.Set("X-Plex-Token", configuredServer.token)
		playURL.RawQuery = query.Encode()
		return playback.Link{
			PlayURL:      playURL.String(),
			ItemName:     item.Title,
			ServerName:   configuredServer.name,
			PlaybackMode: "stream",
		}, nil
	}
	if len(failures) > 0 {
		return playback.Link{}, errors.New(strings.Join(failures, "；"))
	}
	return playback.Link{}, playback.ErrNotFound
}

func (c *Client) findMovie(ctx context.Context, configuredServer server, tmdbID int, queries []string) (*mediaMetadata, error) {
	log.Printf("[Plex] [%s] 开始匹配电影 TMDB ID: %d, 搜索词: %v", configuredServer.name, tmdbID, queries)
	item, err := c.findMovieByHubSearch(ctx, configuredServer, tmdbID, queries)
	if err != nil {
		return nil, err
	}
	if item != nil {
		log.Printf("[Plex] [%s] Hub 搜索成功命中电影: %q (ratingKey=%s)", configuredServer.name, item.Title, item.RatingKey)
		return item, nil
	}
	log.Printf("[Plex] [%s] 未在服务器找到 TMDB ID 为 %d 的电影", configuredServer.name, tmdbID)
	return nil, nil
}

func (c *Client) checkItemMatches(ctx context.Context, configuredServer server, item mediaMetadata, tmdbID int) (*mediaMetadata, bool) {
	if hasTMDBID(item, tmdbID) {
		return &item, true
	}
	if item.RatingKey != "" {
		detail, err := c.metadata(ctx, configuredServer, item.RatingKey)
		if err == nil && detail != nil && hasTMDBID(*detail, tmdbID) {
			return detail, true
		}
	}
	return nil, false
}

func (c *Client) findMovieByHubSearch(ctx context.Context, configuredServer server, tmdbID int, queries []string) (*mediaMetadata, error) {
	return c.findByHubSearch(ctx, configuredServer, tmdbID, "movie", queries)
}

func (c *Client) findByHubSearch(ctx context.Context, configuredServer server, tmdbID int, mediaType string, queries []string) (*mediaMetadata, error) {
	var failures []string
	succeeded := false
	for _, qStr := range queries {
		target := configuredServer.resolve("/hubs/search")
		query := target.Query()
		query.Set("query", qStr)
		query.Set("limit", "5")
		target.RawQuery = query.Encode()

		var result mediaContainerResponse
		if err := c.getJSON(ctx, configuredServer, target, &result); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		succeeded = true
		for _, item := range result.MediaContainer.Metadata {
			if item.Type != "" && item.Type != mediaType {
				continue
			}
			if matched, ok := c.checkItemMatches(ctx, configuredServer, item, tmdbID); ok {
				return matched, nil
			}
		}
		for _, h := range result.MediaContainer.Hub {
			if h.Type != "" && h.Type != mediaType {
				continue
			}
			for _, item := range h.Metadata {
				if item.Type != "" && item.Type != mediaType {
					continue
				}
				if matched, ok := c.checkItemMatches(ctx, configuredServer, item, tmdbID); ok {
					return matched, nil
				}
			}
		}
	}
	if !succeeded && len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "；"))
	}
	return nil, nil
}

func (c *Client) SeriesCatalog(ctx context.Context, mediaRef domain.MediaRef) (SeriesCatalog, error) {
	if !c.Configured() {
		return SeriesCatalog{}, playback.ErrNotConfigured
	}
	if mediaRef.Type != "tv" || mediaRef.ID <= 0 {
		return SeriesCatalog{}, errors.New("需要有效的剧集 TMDB ID")
	}
	queries := c.buildSearchQueries(mediaRef)
	if len(queries) == 0 {
		return SeriesCatalog{}, errors.New("缺少可用于 Plex 搜索的剧集名称")
	}

	failures := make([]string, 0)
	for serverIndex, configuredServer := range c.servers {
		item, err := c.findByHubSearch(ctx, configuredServer, mediaRef.ID, "show", queries)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s 查询失败: %v", configuredServer.name, err))
			continue
		}
		if item == nil || item.RatingKey == "" {
			continue
		}
		catalog, err := c.loadSeriesCatalog(ctx, configuredServer, serverIndex, *item)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s 读取剧集目录失败: %v", configuredServer.name, err))
			continue
		}
		if len(catalog.Seasons) == 0 {
			continue
		}
		return catalog, nil
	}
	if len(failures) > 0 {
		return SeriesCatalog{}, errors.New(strings.Join(failures, "；"))
	}
	return SeriesCatalog{}, playback.ErrNotFound
}

func (c *Client) loadSeriesCatalog(ctx context.Context, configuredServer server, serverIndex int, show mediaMetadata) (SeriesCatalog, error) {
	seasonItems, err := c.children(ctx, configuredServer, show.RatingKey)
	if err != nil {
		return SeriesCatalog{}, err
	}
	catalog := SeriesCatalog{
		SeriesTitle: show.Title,
		ServerID:    strconv.Itoa(serverIndex),
		ServerName:  configuredServer.name,
		SeriesKey:   show.RatingKey,
		Seasons:     make([]Season, 0, len(seasonItems)),
	}
	for _, seasonItem := range seasonItems {
		if (seasonItem.Type != "" && seasonItem.Type != "season") || seasonItem.RatingKey == "" {
			continue
		}
		episodeItems, err := c.children(ctx, configuredServer, seasonItem.RatingKey)
		if err != nil {
			return SeriesCatalog{}, err
		}
		season := Season{Number: seasonItem.Index, Title: seasonItem.Title, Episodes: make([]Episode, 0, len(episodeItems))}
		for _, episodeItem := range episodeItems {
			if (episodeItem.Type != "" && episodeItem.Type != "episode") || episodeItem.RatingKey == "" {
				continue
			}
			seasonNumber := episodeItem.ParentIndex
			if seasonNumber == 0 && seasonItem.Index != 0 {
				seasonNumber = seasonItem.Index
			}
			season.Episodes = append(season.Episodes, Episode{
				RatingKey:     episodeItem.RatingKey,
				SeasonNumber:  seasonNumber,
				EpisodeNumber: episodeItem.Index,
				Title:         episodeItem.Title,
				Duration:      episodeItem.Duration,
				AirDate:       episodeItem.OriginallyAvailableAt,
			})
		}
		if len(season.Episodes) > 0 {
			catalog.Seasons = append(catalog.Seasons, season)
		}
	}
	return catalog, nil
}

func (c *Client) children(ctx context.Context, configuredServer server, ratingKey string) ([]mediaMetadata, error) {
	items := make([]mediaMetadata, 0)
	for page := 0; page < 1000; page++ {
		target := configuredServer.resolve("/library/metadata/" + url.PathEscape(ratingKey) + "/children")
		query := target.Query()
		query.Set("X-Plex-Container-Start", strconv.Itoa(len(items)))
		query.Set("X-Plex-Container-Size", strconv.Itoa(childrenPageSize))
		target.RawQuery = query.Encode()
		var result mediaContainerResponse
		if err := c.getJSON(ctx, configuredServer, target, &result); err != nil {
			return nil, err
		}
		pageItems := result.MediaContainer.Metadata
		items = append(items, pageItems...)
		if len(pageItems) == 0 || result.MediaContainer.TotalSize <= len(items) || len(pageItems) < childrenPageSize {
			return items, nil
		}
	}
	return nil, errors.New("Plex 剧集目录分页超过限制")
}

func (c *Client) EpisodePlayLink(ctx context.Context, serverID, seriesKey, episodeKey string) (playback.Link, error) {
	if !c.Configured() {
		return playback.Link{}, playback.ErrNotConfigured
	}
	serverIndex, err := strconv.Atoi(strings.TrimSpace(serverID))
	if err != nil || serverIndex < 0 || serverIndex >= len(c.servers) || !validRatingKey(seriesKey) || !validRatingKey(episodeKey) {
		return playback.Link{}, ErrInvalidSeriesRequest
	}
	configuredServer := c.servers[serverIndex]
	item, err := c.metadata(ctx, configuredServer, episodeKey)
	if err != nil {
		return playback.Link{}, err
	}
	if item == nil || item.Type != "episode" || item.GrandparentRatingKey != seriesKey {
		return playback.Link{}, playback.ErrNotFound
	}
	partKey := firstPartKey(*item)
	if partKey == "" {
		return playback.Link{}, errors.New("Plex 未返回可播放的媒体 Part")
	}
	playURL := configuredServer.resolve(partKey)
	query := playURL.Query()
	query.Set("X-Plex-Token", configuredServer.token)
	playURL.RawQuery = query.Encode()
	return playback.Link{
		PlayURL:      playURL.String(),
		ItemName:     item.Title,
		ServerName:   configuredServer.name,
		PlaybackMode: "stream",
	}, nil
}

func validRatingKey(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (c *Client) metadata(ctx context.Context, configuredServer server, ratingKey string) (*mediaMetadata, error) {
	target := configuredServer.resolve("/library/metadata/" + url.PathEscape(ratingKey))
	query := target.Query()
	query.Set("includeGuids", "1")
	target.RawQuery = query.Encode()
	var result mediaContainerResponse
	if err := c.getJSON(ctx, configuredServer, target, &result); err != nil {
		return nil, err
	}
	if len(result.MediaContainer.Metadata) == 0 {
		return nil, nil
	}
	return &result.MediaContainer.Metadata[0], nil
}

func (c *Client) getJSON(ctx context.Context, configuredServer server, target *url.URL, output any) error {
	start := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Plex-Token", configuredServer.token)
	request.Header.Set("X-Plex-Product", "Traker")
	request.Header.Set("X-Plex-Client-Identifier", "traker")
	response, err := c.http.Do(request)
	if err != nil {
		log.Printf("[Plex] [%s] HTTP GET %s 失败: %v (耗时 %v)", configuredServer.name, sanitizeURL(target), err, time.Since(start))
		return err
	}
	defer response.Body.Close()
	log.Printf("[Plex] [%s] HTTP GET %s -> %d (耗时 %v)", configuredServer.name, sanitizeURL(target), response.StatusCode, time.Since(start))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(output); err != nil {
		return err
	}
	return nil
}

func sanitizeURL(target *url.URL) string {
	if target == nil {
		return ""
	}
	cloned := *target
	query := cloned.Query()
	if query.Has("X-Plex-Token") {
		query.Set("X-Plex-Token", "***")
		cloned.RawQuery = query.Encode()
	}
	return cloned.String()
}

func (configuredServer server) resolve(key string) *url.URL {
	target := *configuredServer.apiURL
	basePath := strings.TrimRight(target.Path, "/")
	keyPath := key
	if decoded, err := url.PathUnescape(key); err == nil {
		keyPath = decoded
	}
	target.Path = basePath + "/" + strings.TrimLeft(keyPath, "/")
	target.RawPath = ""
	target.RawQuery = ""
	target.Fragment = ""
	return &target
}

func hasTMDBID(item mediaMetadata, tmdbID int) bool {
	id := strconv.Itoa(tmdbID)
	if matchesTMDBGUID(item.GUID, id) {
		return true
	}
	for _, guid := range item.GUIDs {
		if matchesTMDBGUID(guid.ID, id) {
			return true
		}
	}
	return false
}

func matchesTMDBGUID(raw, id string) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	for _, prefix := range []string{"tmdb://", "themoviedb://", "com.plexapp.agents.themoviedb://"} {
		if strings.HasPrefix(normalized, prefix+id) {
			rest := strings.TrimPrefix(normalized, prefix+id)
			return rest == "" || strings.HasPrefix(rest, "?")
		}
	}
	return false
}

func firstPartKey(item mediaMetadata) string {
	for _, media := range item.Media {
		for _, part := range media.Parts {
			if strings.TrimSpace(part.Key) != "" {
				return part.Key
			}
		}
	}
	return ""
}
