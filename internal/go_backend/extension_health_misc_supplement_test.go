package gobackend

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExtensionHealthClassificationAndValidation(t *testing.T) {
	if status, msg := classifyExtensionHealthBody([]byte(`{"status":"degraded"}`), ""); status != "degraded" || msg != "degraded" {
		t.Fatalf("status/message = %q/%q", status, msg)
	}
	if status, _ := classifyExtensionHealthBody([]byte(`not-json`), ""); status != "online" {
		t.Fatalf("invalid JSON status = %q", status)
	}
	if status, msg := classifyExtensionHealthBody([]byte(`{"services":{"tidal":{"status":401,"label":"Tidal","detail":"auth_required"}}}`), "tidal"); status != "degraded" || !strings.Contains(msg, "Tidal") {
		t.Fatalf("service status/message = %q/%q", status, msg)
	}
	if status, msg, ok := classifyExtensionHealthService(map[string]interface{}{"services": map[string]interface{}{}}, "missing"); !ok || status != "unknown" || !strings.Contains(msg, "missing") {
		t.Fatalf("missing service = %q/%q/%v", status, msg, ok)
	}
	if n, ok := healthNumber(json.Number("503")); !ok || n != 503 {
		t.Fatalf("health number = %d/%v", n, ok)
	}
	if !isExtensionHealthAuthRequired(" unauthorized ") {
		t.Fatal("expected auth required")
	}

	if result := CheckExtensionHealth(nil); result.Status != "offline" {
		t.Fatalf("nil health = %#v", result)
	}
	manifest := &ExtensionManifest{Permissions: ExtensionPermissions{Network: []string{"status.example.com"}}}
	invalidURL := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "bad", URL: "://bad"})
	if invalidURL.Status != "offline" {
		t.Fatalf("invalid URL = %#v", invalidURL)
	}
	insecure := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "http", URL: "http://status.example.com"})
	if insecure.Status != "offline" || !strings.Contains(insecure.Error, "https") {
		t.Fatalf("insecure = %#v", insecure)
	}
	disallowedHost := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "host", URL: "https://other.example.com"})
	if disallowedHost.Status != "offline" || !strings.Contains(disallowedHost.Error, "permissions") {
		t.Fatalf("host = %#v", disallowedHost)
	}
	badMethod := runExtensionHealthCheck(manifest, ExtensionHealthCheck{ID: "method", URL: "https://status.example.com", Method: "POST"})
	if badMethod.Status != "offline" || !strings.Contains(badMethod.Error, "method") {
		t.Fatalf("method = %#v", badMethod)
	}

	ext := &loadedExtension{
		ID: "health-ext",
		Manifest: &ExtensionManifest{
			ServiceHealth: []ExtensionHealthCheck{
				{ID: "required", URL: "http://status.example.com", Required: true},
				{ID: "optional", URL: "http://status.example.com", Required: false},
			},
		},
	}
	if result := CheckExtensionHealth(ext); result.Status != "offline" || len(result.Checks) != 2 {
		t.Fatalf("extension health = %#v", result)
	}
}

func TestCoverRomajiParallelAndIDHSHelpers(t *testing.T) {
	spotify := "https://i.scdn.co/image/ab67616d00001e02abcdef"
	if got := GetCoverFromSpotify(spotify, true); !strings.Contains(got, spotifySizeMax) {
		t.Fatalf("spotify cover = %q", got)
	}
	if got := upgradeToMaxQuality("https://cdn-images.dzcdn.net/images/cover/abc/500x500-000000-80-0-0.jpg"); !strings.Contains(got, "1800x1800") {
		t.Fatalf("deezer cover = %q", got)
	}
	if got := upgradeToMaxQuality("https://resources.tidal.com/images/id/320x320.jpg"); !strings.Contains(got, "origin.jpg") {
		t.Fatalf("tidal cover = %q", got)
	}
	if got := upgradeToMaxQuality("https://static.qobuz.com/images/covers/ab/cd/foo_600.jpg"); !strings.Contains(got, "_max.jpg") {
		t.Fatalf("qobuz cover = %q", got)
	}
	if data, err := downloadCoverToMemory("", false); err == nil || data != nil {
		t.Fatalf("expected empty cover error")
	}

	if !ContainsJapanese("カタカナ") || ContainsJapanese("abc") {
		t.Fatal("unexpected Japanese detection")
	}
	if got := JapaneseToRomaji("きゃット"); got != "kyatto" {
		t.Fatalf("romaji = %q", got)
	}
	if got := BuildSearchQuery("きゃ! song", "アーティスト"); got != "atisuto kya song" {
		t.Fatalf("query = %q", got)
	}
	if got := CleanToASCII("A, B. C!"); got != "A B C" {
		t.Fatalf("ascii = %q", got)
	}

	if err := PreWarmCache(`not-json`); err == nil {
		t.Fatal("expected prewarm JSON error")
	}
	if err := PreWarmCache(`[{"isrc":"ISRC","track_name":"Song","artist_name":"Artist","spotify_id":"sp","service":"tidal"}]`); err != nil {
		t.Fatalf("PreWarmCache: %v", err)
	}
	if result := FetchCoverAndLyricsParallel("", false, "", "", "", false, 0); result == nil || result.CoverErr != nil || result.LyricsErr != nil {
		t.Fatalf("parallel result = %#v", result)
	}
	if ClearTrackCache(); GetCacheSize() != 0 {
		t.Fatal("expected empty cache size")
	}

	client := &IDHSClient{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s", req.Method)
		}
		body := `{"id":"1","type":"song","title":"Song","links":[{"type":"tidal","url":"https://tidal.com/browse/track/7"},{"type":"deezer","url":"https://www.deezer.com/track/9"},{"type":"spotify","url":"https://open.spotify.com/track/abc"}]}`
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}}
	availability, err := client.GetAvailabilityFromSpotify("spotify-track")
	if err != nil {
		t.Fatalf("GetAvailabilityFromSpotify: %v", err)
	}
	if !availability.Tidal || !availability.Deezer || availability.DeezerID != "9" {
		t.Fatalf("spotify availability = %#v", availability)
	}
	deezerAvailability, err := client.GetAvailabilityFromDeezer("9")
	if err != nil {
		t.Fatalf("GetAvailabilityFromDeezer: %v", err)
	}
	if deezerAvailability.SpotifyID != "abc" || !deezerAvailability.Tidal {
		t.Fatalf("deezer availability = %#v", deezerAvailability)
	}

	errorClient := &IDHSClient{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})}}
	if _, err := errorClient.Search("bad", nil); err == nil {
		t.Fatal("expected rate limit error")
	}
}
