package playback

import (
	"reflect"
	"testing"
)

func TestParseProviderOrder(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "default", want: []string{"emby", "plex"}},
		{name: "plex first", raw: "plex,emby", want: []string{"plex", "emby"}},
		{name: "case and whitespace", raw: " PLEX, EMBY ", want: []string{"plex", "emby"}},
		{name: "missing provider", raw: "plex", wantErr: true},
		{name: "duplicate provider", raw: "plex,plex", wantErr: true},
		{name: "unknown provider", raw: "plex,jellyfin", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseProviderOrder(test.raw)
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("unexpected order: got %v want %v", got, test.want)
			}
		})
	}
}
