// Package clockify is a tiny read-only client for the Clockify REST API: who
// am I, which workspaces, and my time entries in a range. Nothing here writes.
package clockify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultBaseURL is the global Clockify API endpoint.
const DefaultBaseURL = "https://api.clockify.me/api/v1"

// pageSize per time-entries request; a month of entries fits into one page,
// which matters on free workspaces with a tight hourly request budget.
const pageSize = 1000

// maxPages caps a single range fetch (pageSize × maxPages entries).
const maxPages = 10

// ErrUnauthorized means the API key was rejected.
var ErrUnauthorized = errors.New("clockify: the API key was rejected")

// ErrRateLimited means Clockify answered 429.
var ErrRateLimited = errors.New("clockify: rate limit reached, try again later")

// Client talks to Clockify with one API key. The key acts as its user.
type Client struct {
	Key     string
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for the given key with sane defaults.
func New(key string) *Client {
	return &Client{Key: key, BaseURL: DefaultBaseURL, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// User is the owner of the API key.
type User struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	ActiveWorkspace string `json:"activeWorkspace"`
	Settings        struct {
		TimeZone string `json:"timeZone"`
	} `json:"settings"`
}

// Workspace is one workspace the user belongs to.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Rate is an amount in minor units (cents).
type Rate struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Entry is a time entry as returned with hydrated=true.
type Entry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Billable    bool   `json:"billable"`
	ProjectID   string `json:"projectId"`
	Project     *struct {
		Name       string `json:"name"`
		ClientName string `json:"clientName"`
	} `json:"project"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
	TimeInterval struct {
		Start time.Time  `json:"start"`
		End   *time.Time `json:"end"`
	} `json:"timeInterval"`
	HourlyRate *Rate `json:"hourlyRate"`
}

// Running reports whether the timer of this entry is still going.
func (e Entry) Running() bool { return e.TimeInterval.End == nil }

// Seconds is the tracked duration (0 for a running entry).
func (e Entry) Seconds() int64 {
	if e.Running() {
		return 0
	}
	return int64(e.TimeInterval.End.Sub(e.TimeInterval.Start).Seconds())
}

// User returns the owner of the key.
func (c *Client) User(ctx context.Context) (*User, error) {
	var u User
	if _, err := c.get(ctx, "/user", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Workspaces lists the workspaces the user belongs to.
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	var ws []Workspace
	if _, err := c.get(ctx, "/workspaces", nil, &ws); err != nil {
		return nil, err
	}
	return ws, nil
}

// TimeEntries returns the user's entries that started in [start, end).
// The endpoint is scoped to one user by its path, so it never returns
// anybody else's time even when the key belongs to a workspace admin.
func (c *Client) TimeEntries(ctx context.Context, workspace, user string, start, end time.Time) ([]Entry, error) {
	var all []Entry
	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		q.Set("start", start.UTC().Format("2006-01-02T15:04:05Z"))
		q.Set("end", end.UTC().Format("2006-01-02T15:04:05Z"))
		q.Set("hydrated", "true")
		q.Set("page", strconv.Itoa(page))
		q.Set("page-size", strconv.Itoa(pageSize))
		var batch []Entry
		h, err := c.get(ctx, "/workspaces/"+url.PathEscape(workspace)+"/user/"+url.PathEscape(user)+"/time-entries", q, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if h.Get("Last-Page") == "true" || len(batch) < pageSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("clockify: more than %d entries in that range", pageSize*maxPages)
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) (http.Header, error) {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.Key)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clockify: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, ErrUnauthorized
	case res.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case res.StatusCode >= 300:
		body, _ := io.ReadAll(io.LimitReader(res.Body, 500))
		return nil, fmt.Errorf("clockify: %s: %s", res.Status, body)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("clockify: decoding %s: %w", path, err)
	}
	return res.Header, nil
}

// DayRange turns inclusive calendar dates (YYYY-MM-DD) in the given time zone
// into [start, end): from 00:00 on the first day to 00:00 after the last.
func DayRange(from, to, tz string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		loc = time.UTC
	}
	start, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid from date %q", from)
	}
	last, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid to date %q", to)
	}
	if last.Before(start) {
		return time.Time{}, time.Time{}, errors.New("the to date is before the from date")
	}
	return start, last.AddDate(0, 0, 1), nil
}
