package scala

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

const maxResponseSize = 4 << 20

type Error struct {
	Message   string
	Retryable bool
}

func (err *Error) Error() string   { return err.Message }
func (err *Error) Temporary() bool { return err.Retryable }

type Client struct {
	endpoint string
	http     *http.Client
}

func NewClient(endpoint string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("SCALA_ANALYSIS_URL must be an absolute HTTP(S) URL without credentials or fragment")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("Scala timeout must be positive")
	}
	return &Client{endpoint: endpoint, http: &http.Client{
		Timeout: timeout,
		// A redirect must never turn an analysis POST into a GET.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (client *Client) Analyze(ctx context.Context, batch domain.AnalysisRequest) (domain.AnalysisResult, error) {
	var result domain.AnalysisResult
	body, err := json.Marshal(batch)
	if err != nil {
		return result, &Error{Message: "encode analysis request: " + err.Error()}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return result, &Error{Message: "create analysis request: " + err.Error()}
	}
	digest := sha256.Sum256(body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", hex.EncodeToString(digest[:]))
	response, err := client.http.Do(request)
	if err != nil {
		return result, &Error{Message: "send analysis request: " + err.Error(), Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// Do not retain an arbitrary upstream body in public status responses.
		return result, &Error{
			Message:   fmt.Sprintf("Scala returned HTTP %d; expected 200 with an analysis result", response.StatusCode),
			Retryable: response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
		}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return result, &Error{Message: "Scala response Content-Type must be application/json"}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return result, &Error{Message: "read Scala response: " + err.Error(), Retryable: true}
	}
	if len(data) > maxResponseSize {
		return result, &Error{Message: "Scala response exceeds 4 MiB"}
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, &Error{Message: "Scala response is not a valid analysis JSON: " + err.Error()}
	}
	if result.MatchID != batch.MatchID || result.GameID != batch.GameID {
		return result, &Error{Message: "Scala response matchId/gameId do not match the submitted match"}
	}
	if !hasShape(result.PlayerStats, '{') || !hasShape(result.MatchStats, '{') || !hasShape(result.Patterns, '[') {
		return result, &Error{Message: "Scala must return playerStats and matchStats objects, and a patterns array"}
	}
	return result, nil
}

func hasShape(data json.RawMessage, first byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == first && json.Valid(trimmed)
}
