// Package logbook queries the HA /api/logbook endpoint. The package exists
// because HA rejects "+00:00" in the URL timestamp (URL-encoding of "+" breaks
// the parse), so timestamps must be serialized with the "Z" suffix. Centralizing
// that here prevents callers from getting it wrong.
package logbook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Event is one row from the logbook response.
type Event struct {
	When        time.Time `json:"when"`
	State       string    `json:"state"`
	EntityID    string    `json:"entity_id"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon,omitempty"`
	ContextName string    `json:"context_name,omitempty"`
}

// Query fetches logbook events for the given entities in [start, end].
// baseURL should be the HA root (no trailing slash, but stripped if present).
func Query(httpClient *http.Client, baseURL, token string, entityIDs []string, start, end time.Time) ([]Event, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse baseURL: %w", err)
	}
	// HA requires the "Z" suffix here; "+00:00" breaks because URL-encoding "+" yields a literal '+'.
	u.Path += "/api/logbook/" + start.UTC().Format("2006-01-02T15:04:05Z")
	q := u.Query()
	q.Set("end_time", end.UTC().Format("2006-01-02T15:04:05Z"))
	q.Set("entity", strings.Join(entityIDs, ","))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HA returned %d: %s", resp.StatusCode, string(body))
	}

	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("decode response: %w (body: %s)", err, string(body))
	}
	return events, nil
}
