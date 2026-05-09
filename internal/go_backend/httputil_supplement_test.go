package gobackend

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTTPUtilityHelpers(t *testing.T) {
	SetAppVersion("7.0.0")
	apiURL := mustParseURL(t, "https://api.zarz.moe/test")
	if ua := userAgentForURL(apiURL); !strings.Contains(ua, "7.0.0") {
		t.Fatalf("api user agent = %q", ua)
	}
	if userAgentForURL(nil) == "" || userAgentForURL(mustParseURL(t, "https://example.com")) == "" {
		t.Fatal("expected fallback user agent")
	}
	if NewHTTPClientWithTimeout(time.Second).Timeout != time.Second || NewMetadataHTTPClient(time.Second).Timeout != time.Second {
		t.Fatal("client timeout mismatch")
	}
	if GetSharedClient() == nil || GetDownloadClient() == nil {
		t.Fatal("expected shared clients")
	}
	SetNetworkCompatibilityOptions(true, true)
	if opts := GetNetworkCompatibilityOptions(); !opts.AllowHTTP || !opts.InsecureTLS {
		t.Fatalf("network opts = %#v", opts)
	}
	SetNetworkCompatibilityOptions(false, false)
	if !canFallbackToHTTP(&http.Request{Method: http.MethodGet}) {
		t.Fatal("GET should fallback")
	}
	if canFallbackToHTTP(&http.Request{Method: http.MethodPost}) {
		t.Fatal("POST without GetBody should not fallback")
	}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/path", strings.NewReader("body"))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("body")), nil }
	cloned, err := cloneRequestWithHTTPScheme(req, "http")
	if err != nil || cloned.URL.Scheme != "http" || cloned.Body == nil {
		t.Fatalf("cloneRequestWithHTTPScheme = %#v/%v", cloned, err)
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("User-Agent") == "" {
			t.Fatal("missing User-Agent")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
	})}
	resp, err := DoRequestWithUserAgent(client, mustNewRequest(t, "https://example.com/ok"))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("DoRequestWithUserAgent = %#v/%v", resp, err)
	}
	resp.Body.Close()

	attempts := 0
	retryClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		switch attempts {
		case 1:
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("server")), Request: req}, nil
		case 2:
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader("rate")), Request: req}, nil
		default:
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		}
	})}
	resp, err = DoRequestWithRetry(retryClient, mustNewRequest(t, "https://example.com/retry"), RetryConfig{MaxRetries: 3, InitialDelay: 0, MaxDelay: time.Millisecond, BackoffFactor: 2})
	if err != nil || resp.StatusCode != 204 || attempts != 3 {
		t.Fatalf("DoRequestWithRetry = %#v/%v attempts=%d", resp, err, attempts)
	}
	resp.Body.Close()
	blockingClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("access denied by region")), Request: req}, nil
	})}
	if _, err := DoRequestWithRetry(blockingClient, mustNewRequest(t, "https://blocked.example.com"), RetryConfig{MaxRetries: 0}); err == nil {
		t.Fatal("expected blocking retry error")
	}

	if _, err := ReadResponseBody(nil); err == nil {
		t.Fatal("expected nil response body error")
	}
	if _, err := ReadResponseBody(&http.Response{Body: io.NopCloser(strings.NewReader(""))}); err == nil {
		t.Fatal("expected empty response body error")
	}
	if body, err := ReadResponseBody(&http.Response{Body: io.NopCloser(strings.NewReader("ok"))}); err != nil || string(body) != "ok" {
		t.Fatalf("ReadResponseBody = %q/%v", body, err)
	}
	if err := ValidateResponse(nil); err == nil {
		t.Fatal("expected nil response validation error")
	}
	if err := ValidateResponse(&http.Response{StatusCode: 404, Status: "404 Not Found"}); err == nil {
		t.Fatal("expected bad status validation error")
	}
	if err := ValidateResponse(&http.Response{StatusCode: 200}); err != nil {
		t.Fatalf("ValidateResponse: %v", err)
	}
	if msg := BuildErrorMessage("api", 500, strings.Repeat("x", 120)); !strings.Contains(msg, "...") {
		t.Fatalf("BuildErrorMessage = %q", msg)
	}
	if calculateNextDelay(10*time.Millisecond, RetryConfig{BackoffFactor: 3, MaxDelay: 20 * time.Millisecond}) != 20*time.Millisecond {
		t.Fatal("calculateNextDelay mismatch")
	}
	if getRetryAfterDuration(&http.Response{Header: http.Header{"Retry-After": []string{"bad"}}}) != 0 {
		t.Fatal("invalid retry-after should be zero")
	}
	if isp := IsISPBlocking(errors.New("connection reset by peer"), "https://example.com/x"); isp == nil || !strings.Contains(isp.Error(), "example.com") {
		t.Fatalf("IsISPBlocking = %#v", isp)
	}
	if !CheckAndLogISPBlocking(errors.New("i/o timeout"), "https://timeout.example/x", "test") {
		t.Fatal("expected logged ISP blocking")
	}
	if wrapped := WrapErrorWithISPCheck(errors.New("connection refused"), "https://refused.example/x", "test"); wrapped == nil || !strings.Contains(wrapped.Error(), "ISP blocking") {
		t.Fatalf("WrapErrorWithISPCheck = %v", wrapped)
	}
	if WrapErrorWithISPCheck(nil, "", "test") != nil {
		t.Fatal("nil wrap should stay nil")
	}
	if extractDomain("https://example.com/path") != "example.com" || extractDomain("bad://") != "unknown" || extractDomain("") != "unknown" {
		t.Fatal("extractDomain mismatch")
	}
}

func TestRateLimiterHelpers(t *testing.T) {
	limiter := NewRateLimiter(1, time.Hour)
	if limiter.Available() != 1 {
		t.Fatalf("available = %d", limiter.Available())
	}
	if !limiter.TryAcquire() || limiter.TryAcquire() {
		t.Fatal("TryAcquire mismatch")
	}
	if limiter.Available() != 0 {
		t.Fatalf("available after acquire = %d", limiter.Available())
	}
	if GetSongLinkRateLimiter() == nil {
		t.Fatal("expected global limiter")
	}
}

func mustNewRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
