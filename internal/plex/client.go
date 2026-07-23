package plex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"traker/internal/domain"
	"traker/internal/playback"
)

const pageSize = 200

type ServerConfig struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

type Client struct {
	servers []server
	http    *http.Client
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
	} `json:"MediaContainer"`
}

type directory struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type mediaMetadata struct {
	RatingKey string `json:"ratingKey"`
	Title     string `json:"title"`
	GUID      string `json:"guid"`
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

	failures := make([]string, 0)
	for _, configuredServer := range c.servers {
		item, err := c.findMovie(ctx, configuredServer, mediaRef.ID)
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

func (c *Client) findMovie(ctx context.Context, configuredServer server, tmdbID int) (*mediaMetadata, error) {
	var sections mediaContainerResponse
	if err := c.getJSON(ctx, configuredServer, configuredServer.resolve("/library/sections"), &sections); err != nil {
		return nil, err
	}
	for _, section := range sections.MediaContainer.Directory {
		if section.Type != "movie" || section.Key == "" {
			continue
		}
		item, err := c.findMovieInSection(ctx, configuredServer, section.Key, tmdbID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", section.Title, err)
		}
		if item != nil {
			return item, nil
		}
	}
	return nil, nil
}

func (c *Client) findMovieInSection(ctx context.Context, configuredServer server, sectionKey string, tmdbID int) (*mediaMetadata, error) {
	start := 0
	for page := 0; page < 1000; page++ {
		target := configuredServer.resolve("/library/sections/" + url.PathEscape(sectionKey) + "/all")
		query := target.Query()
		query.Set("type", "1")
		query.Set("includeGuids", "1")
		query.Set("X-Plex-Container-Start", strconv.Itoa(start))
		query.Set("X-Plex-Container-Size", strconv.Itoa(pageSize))
		target.RawQuery = query.Encode()

		var result mediaContainerResponse
		if err := c.getJSON(ctx, configuredServer, target, &result); err != nil {
			return nil, err
		}
		items := result.MediaContainer.Metadata
		for index := range items {
			if hasTMDBID(items[index], tmdbID) {
				return &items[index], nil
			}
		}
		if len(items) == 0 {
			return nil, nil
		}
		start += len(items)
		if result.MediaContainer.TotalSize > 0 && start >= result.MediaContainer.TotalSize {
			return nil, nil
		}
		if len(items) < pageSize {
			return nil, nil
		}
	}
	return nil, errors.New("Plex 媒体库分页超过限制")
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
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(output); err != nil {
		return err
	}
	return nil
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
