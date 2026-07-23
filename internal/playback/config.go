package playback

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	ProviderEmby = "emby"
	ProviderPlex = "plex"
)

var defaultProviderOrder = []string{ProviderEmby, ProviderPlex}

func ProviderOrderFromEnvironment() ([]string, error) {
	return ParseProviderOrder(os.Getenv("PLAYBACK_PROVIDER_ORDER"))
}

func ParseProviderOrder(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), defaultProviderOrder...), nil
	}

	parts := strings.Split(raw, ",")
	if len(parts) != len(defaultProviderOrder) {
		return nil, errors.New("PLAYBACK_PROVIDER_ORDER 必须同时包含 emby 和 plex")
	}
	order := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		name := strings.ToLower(strings.TrimSpace(part))
		if name != ProviderEmby && name != ProviderPlex {
			return nil, fmt.Errorf("PLAYBACK_PROVIDER_ORDER 包含未知来源 %q", strings.TrimSpace(part))
		}
		if seen[name] {
			return nil, fmt.Errorf("PLAYBACK_PROVIDER_ORDER 重复配置 %q", name)
		}
		seen[name] = true
		order = append(order, name)
	}
	for _, name := range defaultProviderOrder {
		if !seen[name] {
			return nil, fmt.Errorf("PLAYBACK_PROVIDER_ORDER 缺少 %q", name)
		}
	}
	return order, nil
}
