package ocrworker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	allowedHostSuffix = "gamejob.co.kr"
	refererURL        = "https://www.gamejob.co.kr/"
	downloadTimeout   = 15 * time.Second
)

// imageMagicPrefixes guards against Content-Type spoofing: even if the server
// claims image/jpeg, the response bytes must start with a recognized image
// magic number.
var imageMagicPrefixes = [][]byte{
	{0xff, 0xd8, 0xff},
	{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
	[]byte("GIF87a"),
	[]byte("GIF89a"),
	[]byte("RIFF"),
	[]byte("BM"),
}

var (
	errHostNotAllowed = errors.New("host not allowed")
	errNotImage       = errors.New("response is not an image")
)

func hostAllowed(rawURL string) (bool, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("parse url: %w", err)
	}
	host := strings.ToLower(parsed.Hostname())
	return host == allowedHostSuffix || strings.HasSuffix(host, "."+allowedHostSuffix), nil
}

func looksLikeImage(payload []byte) bool {
	for _, prefix := range imageMagicPrefixes {
		if bytes.HasPrefix(payload, prefix) {
			return true
		}
	}
	return false
}

// downloadImage fetches an image from a gamejob host. Returns the raw bytes,
// or an error if the host is not allowed, the request fails, or the response
// fails the image content-type/magic-byte check.
func downloadImage(ctx context.Context, rawURL string) ([]byte, error) {
	allowed, err := hostAllowed(rawURL)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("%w: %s", errHostNotAllowed, rawURL)
	}

	reqCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Referer", refererURL)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
	if !strings.HasPrefix(contentType, "image/") && !looksLikeImage(payload) {
		return nil, fmt.Errorf("%w: ct=%q bytes=%d", errNotImage, contentType, len(payload))
	}

	return payload, nil
}
