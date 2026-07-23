package playback

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"traker/internal/domain"
)

var (
	ErrNotConfigured = errors.New("播放服务尚未配置")
	ErrNotFound      = errors.New("媒体库中未找到对应影片")
	ErrUnsupported   = errors.New("剧集需要指定具体集数，暂不支持直接播放")
)

type Link struct {
	PlayURL       string `json:"playUrl"`
	RedirectedURL string `json:"redirectedUrl"`
	ItemName      string `json:"itemName"`
	ServerName    string `json:"serverName"`
	PlaybackMode  string `json:"playbackMode"`
}

type Provider interface {
	Configured() bool
	PlayLink(context.Context, domain.MediaRef) (Link, error)
}

type Client struct {
	providers []Provider
}

func NewClient(providers ...Provider) *Client {
	return &Client{providers: providers}
}

func (c *Client) Configured() bool {
	if c == nil {
		return false
	}
	for _, provider := range c.providers {
		if provider != nil && provider.Configured() {
			return true
		}
	}
	return false
}

func (c *Client) PlayLink(ctx context.Context, mediaRef domain.MediaRef) (Link, error) {
	if !c.Configured() {
		return Link{}, ErrNotConfigured
	}
	if (mediaRef.Type != "tm" && mediaRef.Type != "tv") || mediaRef.ID <= 0 {
		return Link{}, errors.New("无效的 TMDB ID")
	}
	if mediaRef.Type == "tv" {
		return Link{}, ErrUnsupported
	}

	failures := make([]string, 0)
	for _, provider := range c.providers {
		if provider == nil || !provider.Configured() {
			continue
		}
		link, err := provider.PlayLink(ctx, mediaRef)
		if err == nil {
			return link, nil
		}
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotConfigured) {
			continue
		}
		if errors.Is(err, ErrUnsupported) {
			return Link{}, err
		}
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return Link{}, fmt.Errorf("播放服务查询失败: %s", strings.Join(failures, "；"))
	}
	return Link{}, ErrNotFound
}
