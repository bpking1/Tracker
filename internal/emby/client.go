package emby

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
)

var (
	ErrNotConfigured = errors.New("Emby 尚未配置")
	ErrNotFound      = errors.New("Emby 媒体库中未找到对应影片")
	ErrUnsupported   = errors.New("剧集需要指定具体集数，暂不支持直接播放")
)

type ServerConfig struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

type PlayLink struct {
	PlayURL       string `json:"playUrl"`
	RedirectedURL string `json:"redirectedUrl"`
	ItemName      string `json:"itemName"`
	ServerName    string `json:"serverName"`
	PlaybackMode  string `json:"playbackMode"`
}

type Client struct {
	servers []server
	http    *http.Client
}

type server struct {
	name   string
	apiURL *url.URL
	apiKey string
}

type searchResult struct {
	Items []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	} `json:"Items"`
}

func NewClientFromEnvironment() (*Client, error) {
	raw := strings.TrimSpace(os.Getenv("EMBY_SERVERS"))
	if raw == "" {
		return &Client{http: defaultHTTPClient()}, nil
	}
	var configs []ServerConfig
	if err := json.Unmarshal([]byte(raw), &configs); err != nil {
		return nil, fmt.Errorf("EMBY_SERVERS 必须是 JSON 数组: %w", err)
	}
	return NewClient(configs, nil)
}

func NewClient(configs []ServerConfig, httpClient *http.Client) (*Client, error) {
	servers := make([]server, 0, len(configs))
	for index, config := range configs {
		apiURL, err := parseServerURL(config.URL)
		if err != nil {
			return nil, fmt.Errorf("Emby 服务 %d 地址无效: %w", index+1, err)
		}
		if strings.TrimSpace(config.APIKey) == "" {
			return nil, fmt.Errorf("Emby 服务 %d 缺少 API Key", index+1)
		}
		name := strings.TrimSpace(config.Name)
		if name == "" {
			name = fmt.Sprintf("Emby %d", index+1)
		}
		servers = append(servers, server{name: name, apiURL: apiURL, apiKey: config.APIKey})
	}
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	return &Client{servers: servers, http: httpClient}, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 12 * time.Second}
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

	apiURL := *parsed
	cleanPath := strings.TrimRight(apiURL.Path, "/")
	if !strings.HasSuffix(strings.ToLower(cleanPath), "/emby") {
		apiURL.Path = cleanPath + "/emby"
	} else {
		apiURL.Path = cleanPath
	}
	return &apiURL, nil
}

func (c *Client) Configured() bool {
	return c != nil && len(c.servers) > 0
}

func (c *Client) PlayLink(ctx context.Context, mediaRef domain.MediaRef) (PlayLink, error) {
	if !c.Configured() {
		return PlayLink{}, ErrNotConfigured
	}
	if (mediaRef.Type != "tm" && mediaRef.Type != "tv") || mediaRef.ID <= 0 {
		return PlayLink{}, errors.New("无效的 TMDB ID")
	}
	if mediaRef.Type == "tv" {
		return PlayLink{}, ErrUnsupported
	}

	failures := make([]string, 0)
	for _, configuredServer := range c.servers {
		item, err := c.search(ctx, configuredServer, "Movie", mediaRef.ID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s 查询失败", configuredServer.name))
			continue
		}
		if item == nil {
			continue
		}
		playURL := streamURL(configuredServer, item.ID)
		redirectURL, err := c.resolveRedirect(ctx, playURL)
		if err != nil {
			return PlayLink{}, fmt.Errorf("%s 播放地址不可用: %w", configuredServer.name, err)
		}
		return PlayLink{
			PlayURL:       playURL,
			RedirectedURL: redirectURL,
			ItemName:      item.Name,
			ServerName:    configuredServer.name,
			PlaybackMode:  "stream",
		}, nil
	}
	if len(failures) > 0 {
		return PlayLink{}, errors.New(strings.Join(failures, "；"))
	}
	return PlayLink{}, ErrNotFound
}

func (c *Client) search(ctx context.Context, configuredServer server, itemType string, tmdbID int) (*struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}, error) {
	searchURL := *configuredServer.apiURL
	searchURL.Path = strings.TrimRight(searchURL.Path, "/") + "/Items"
	query := searchURL.Query()
	query.Set("IncludeItemTypes", itemType)
	query.Set("Recursive", "true")
	query.Set("AnyProviderIdEquals", "tmdb."+strconv.Itoa(tmdbID))
	query.Set("Fields", "Id,Name")
	searchURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Emby-Token", configuredServer.apiKey)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	var result searchResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Items) == 0 || result.Items[0].ID == "" {
		return nil, nil
	}
	return &result.Items[0], nil
}

func streamURL(configuredServer server, itemID string) string {
	target := *configuredServer.apiURL
	target.Path = strings.TrimRight(target.Path, "/") + "/Videos/" + url.PathEscape(itemID) + "/stream"
	query := target.Query()
	query.Set("api_key", configuredServer.apiKey)
	query.Set("static", "true")
	target.RawQuery = query.Encode()
	return target.String()
}

func (c *Client) resolveRedirect(ctx context.Context, playURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, playURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TrakerVideoPlayer)")
	probeClient := *c.http
	probeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := probeClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		location, err := response.Location()
		if err != nil {
			return "", err
		}
		return location.String(), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return "", nil
}
