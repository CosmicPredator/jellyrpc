// Package litterbox uploads files to litterbox.catbox.moe, a free,
// anonymous, temporary file host. We use it as a re-hosting hop for
// Jellyfin artwork when JellyfinURL isn't reachable from the public
// internet (Discord's servers need a public URL to resolve external Rich
// Presence images; they can't be handed a private 192.168.x/10.x address).
//
// Uploads are anonymous and public: anyone with the link can view the
// image for its lifetime. That's normally fine for a movie poster or album
// cover, but keep it in mind before uploading anything sensitive.
package litterbox

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const apiURL = "https://litterbox.catbox.moe/resources/internals/api.php"

// Time is how long the uploaded file stays available. Valid values per
// Litterbox's API: "1h", "12h", "24h", "72h".
type Time string

const (
	Hour1  Time = "1h"
	Hour12 Time = "12h"
	Hour24 Time = "24h"
	Hour72 Time = "72h"
)

// Duration returns the approximate real-world duration for a Time value,
// useful for setting a local cache expiry a bit shorter than the actual
// link lifetime.
func (t Time) Duration() time.Duration {
	switch t {
	case Hour1:
		return time.Hour
	case Hour12:
		return 12 * time.Hour
	case Hour24:
		return 24 * time.Hour
	case Hour72:
		return 72 * time.Hour
	default:
		return time.Hour
	}
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Upload sends data (named filename, only used for its extension/content
// type hint) to Litterbox and returns the resulting public URL.
func Upload(data []byte, filename string, ttl Time) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("reqtype", "fileupload"); err != nil {
		return "", err
	}
	if err := writer.WriteField("time", string(ttl)); err != nil {
		return "", err
	}
	part, err := writer.CreateFormFile("fileToUpload", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("litterbox request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading litterbox response: %w", err)
	}

	url := strings.TrimSpace(string(respBody))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(url, "https://litter.catbox.moe/") {
		return "", fmt.Errorf("litterbox upload failed (status %d): %s", resp.StatusCode, url)
	}

	return url, nil
}