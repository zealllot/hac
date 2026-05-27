package logbook_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zealllot/hac/internal/logbook"
)

func TestQuery_urlUsesZSuffixNotPlusZeroZero(t *testing.T) {
	// The whole point of this package: HA rejects "+00:00" in the logbook URL
	// because URL-encoding of "+" breaks the timestamp. Must serialize as "Z".
	var got *url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	start := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)

	_, err := logbook.Query(srv.Client(), srv.URL, "tok", []string{"light.x"}, start, end)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Path must contain the raw start timestamp with Z suffix (no URL encoding
	// applied to the path segment — that's where today's bug lived).
	if !strings.Contains(got.Path, "2026-05-26T10:00:00Z") {
		t.Errorf("path missing raw start with Z suffix\nactual path: %s", got.Path)
	}
	// Decoded end_time query param must be the Z-suffix form (HA receives the
	// decoded value; the wire-level %3A is irrelevant).
	if endTime := got.Query().Get("end_time"); endTime != "2026-05-27T00:00:00Z" {
		t.Errorf("end_time = %q, want 2026-05-27T00:00:00Z", endTime)
	}
	// Nowhere in either path or raw query may "+" appear (the actual breakage
	// from today's morning curl session).
	raw := got.String()
	if strings.Contains(raw, "+") || strings.Contains(raw, "%2B") {
		t.Errorf("URL contains '+' (HA path parser would fail)\nactual: %s", raw)
	}
}

func TestQuery_passesAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	_, _ = logbook.Query(srv.Client(), srv.URL, "abc-token", []string{"light.x"},
		time.Now().Add(-time.Hour), time.Now())

	if gotAuth != "Bearer abc-token" {
		t.Errorf("Authorization = %q, want 'Bearer abc-token'", gotAuth)
	}
}

func TestQuery_joinsEntityIDsWithComma(t *testing.T) {
	var capturedEntity string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedEntity = r.URL.Query().Get("entity")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	_, _ = logbook.Query(srv.Client(), srv.URL, "tok",
		[]string{"light.a", "light.b", "binary_sensor.c"},
		time.Now().Add(-time.Hour), time.Now())

	want := "light.a,light.b,binary_sensor.c"
	if capturedEntity != want {
		t.Errorf("entity query = %q, want %q", capturedEntity, want)
	}
}

func TestQuery_decodesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"when": "2026-05-26T13:39:03.877Z",
				"state": "off",
				"entity_id": "light.x",
				"name": "灯 X",
				"context_name": "客厅_无人_关灯"
			}
		]`))
	}))
	defer srv.Close()

	events, err := logbook.Query(srv.Client(), srv.URL, "tok", []string{"light.x"},
		time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if e.EntityID != "light.x" || e.State != "off" || e.Name != "灯 X" || e.ContextName != "客厅_无人_关灯" {
		t.Errorf("event fields wrong: %+v", e)
	}
	if e.When.IsZero() {
		t.Error("When was not parsed")
	}
}
