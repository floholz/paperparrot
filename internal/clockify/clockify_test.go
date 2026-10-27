package clockify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDayRange(t *testing.T) {
	start, end, err := DayRange("2026-09-13", "2026-10-26", "Europe/Vienna")
	if err != nil {
		t.Fatal(err)
	}
	// 13.09. is CEST (UTC+2), 27.10. 00:00 is CET (UTC+1) after the DST switch.
	if got := start.UTC().Format(time.RFC3339); got != "2026-09-12T22:00:00Z" {
		t.Errorf("start = %s", got)
	}
	if got := end.UTC().Format(time.RFC3339); got != "2026-10-26T23:00:00Z" {
		t.Errorf("end = %s", got)
	}
	if _, _, err := DayRange("2026-10-02", "2026-10-01", "UTC"); err == nil {
		t.Error("expected error for reversed range")
	}
}

func TestTimeEntriesPaging(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Api-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("non-GET request: %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/workspaces/ws/user/u1/time-entries") {
			t.Errorf("path = %s", r.URL.Path)
		}
		page := r.URL.Query().Get("page")
		if page == "1" {
			// a full page without the header: the client must ask for page 2
			var b strings.Builder
			b.WriteString("[")
			for i := 0; i < pageSize; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"id":"e%d","timeInterval":{"start":"2026-09-01T08:00:00Z","end":"2026-09-01T08:00:36Z"}}`, i)
			}
			b.WriteString("]")
			w.Write([]byte(b.String()))
			return
		}
		w.Header().Set("Last-Page", "true")
		w.Write([]byte(`[{"id":"run","timeInterval":{"start":"2026-09-02T08:00:00Z","end":null}}]`))
	}))
	defer srv.Close()

	c := New("k")
	c.BaseURL = srv.URL
	entries, err := c.TimeEntries(context.Background(), "ws", "u1", time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(entries) != pageSize+1 {
		t.Fatalf("calls=%d entries=%d", calls, len(entries))
	}
	if entries[0].Seconds() != 36 || !entries[pageSize].Running() || entries[pageSize].Seconds() != 0 {
		t.Errorf("unexpected durations: %d running=%v", entries[0].Seconds(), entries[pageSize].Running())
	}

	c.Key = "wrong"
	if _, err := c.User(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New("k")
	c.BaseURL = srv.URL
	if _, err := c.Workspaces(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
}
