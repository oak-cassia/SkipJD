package ocrworker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"skipjd/internal/retry"
)

const (
	browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	allowedHostSuffix  = "gamejob.co.kr"
	refererURL         = "https://www.gamejob.co.kr/"
	downloadTimeout    = 15 * time.Second
	httpRetryAttempts  = 3
	httpRetryBaseDelay = 500 * time.Millisecond
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

// statusErr captures a non-2xx HTTP response
type statusErr struct {
	status int
	msg    string
}

func (e statusErr) Error() string { return e.msg }

func isRetryableImageHTTP(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errHostNotAllowed) || errors.Is(err, errNotImage) {
		return false
	}
	var s statusErr
	if errors.As(err, &s) {
		switch {
		case s.status == http.StatusRequestTimeout, s.status == http.StatusTooManyRequests:
			return true
		case s.status >= 500 && s.status <= 599:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

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

	var payload []byte

	retryErr := retry.Do(ctx, httpRetryAttempts, httpRetryBaseDelay,
		func(attemptCtx context.Context) error {
			reqCtx, cancel := context.WithTimeout(attemptCtx, downloadTimeout)
			defer cancel()

			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
			if err != nil {
				// Don't retry if request build fails
				return fmt.Errorf("build request: %w", err)
			}
			req.Header.Set("User-Agent", browserUserAgent)
			req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
			req.Header.Set("Referer", refererURL)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("http get: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return statusErr{status: resp.StatusCode, msg: fmt.Sprintf("status %d", resp.StatusCode)}
			}

			payloadBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read body: %w", err)
			}

			contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
			if !strings.HasPrefix(contentType, "image/") && !looksLikeImage(payloadBytes) {
				return fmt.Errorf("%w: ct=%q bytes=%d", errNotImage, contentType, len(payloadBytes))
			}

			payload = payloadBytes
			return nil
		},
		isRetryableImageHTTP,
	)

	if retryErr != nil {
		return nil, retryErr
	}
	return payload, nil
}
