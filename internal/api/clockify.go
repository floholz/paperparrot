package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/paperparrot/internal/clockify"
)

// Clockify routes: a per-user connection (settings) and a read-only fetch of
// the user's own time entries for a date range. Filtering and summing happen
// in the browser, so changing a filter costs no further Clockify requests.

// newClockify is swapped in tests.
var newClockify = clockify.New

type clockifySettings struct {
	Connected  bool                 `json:"connected"`
	Name       string               `json:"name,omitempty"`
	Timezone   string               `json:"timezone,omitempty"`
	Workspace  string               `json:"workspace,omitempty"`
	Workspaces []clockify.Workspace `json:"workspaces,omitempty"`
	Rate       float64              `json:"rate"`
}

type clockifyUpdate struct {
	APIKey    *string  `json:"api_key"`
	Workspace *string  `json:"workspace"`
	Rate      *float64 `json:"rate"`
}

// clockifyEntry is the slimmed-down entry sent to the browser.
type clockifyEntry struct {
	Start       string   `json:"start"`
	Seconds     int64    `json:"seconds"`
	Billable    bool     `json:"billable"`
	Project     string   `json:"project"`
	ProjectName string   `json:"project_name"`
	Client      string   `json:"client"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type clockifyEntries struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	Timezone string          `json:"timezone"`
	Entries  []clockifyEntry `json:"entries"`
	Running  int             `json:"running"`
	// ClockifyRate is the most common non-zero hourly rate still stored on the
	// entries (free plans hide rates in the UI, the API keeps returning them).
	ClockifyRate *float64 `json:"clockify_rate"`
	Currency     string   `json:"currency,omitempty"`
}

func clockifyRecord(app core.App, userId string) (*core.Record, error) {
	return app.FindFirstRecordByFilter("clockify", "user = {:u}", dbx.Params{"u": userId})
}

func settingsOf(rec *core.Record) clockifySettings {
	if rec == nil {
		return clockifySettings{}
	}
	var ws []clockify.Workspace
	_ = rec.UnmarshalJSONField("workspaces", &ws)
	return clockifySettings{
		Connected:  true,
		Name:       rec.GetString("clockify_name"),
		Timezone:   rec.GetString("timezone"),
		Workspace:  rec.GetString("workspace"),
		Workspaces: ws,
		Rate:       rec.GetFloat("rate"),
	}
}

func (s *Server) getClockify(re *core.RequestEvent) error {
	rec, _ := clockifyRecord(re.App, re.Auth.Id)
	return re.JSON(http.StatusOK, settingsOf(rec))
}

// putClockify saves the connection. A new API key is checked against
// Clockify right away (2 requests: /user and /workspaces).
func (s *Server) putClockify(re *core.RequestEvent) error {
	var body clockifyUpdate
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("Invalid request body.", err)
	}
	rec, _ := clockifyRecord(re.App, re.Auth.Id)
	if body.APIKey != nil && strings.TrimSpace(*body.APIKey) != "" {
		key := strings.TrimSpace(*body.APIKey)
		c := newClockify(key)
		ctx := re.Request.Context()
		u, err := c.User(ctx)
		if err != nil {
			return clockifyError(re, err)
		}
		ws, err := c.Workspaces(ctx)
		if err != nil {
			return clockifyError(re, err)
		}
		if rec == nil {
			col, err := re.App.FindCollectionByNameOrId("clockify")
			if err != nil {
				return err
			}
			rec = core.NewRecord(col)
			rec.Set("user", re.Auth.Id)
		}
		rec.Set("api_key", key)
		rec.Set("clockify_user", u.ID)
		rec.Set("clockify_name", u.Name)
		rec.Set("timezone", u.Settings.TimeZone)
		rec.Set("workspaces", ws)
		if cur := rec.GetString("workspace"); cur == "" || !hasWorkspace(ws, cur) {
			rec.Set("workspace", u.ActiveWorkspace)
		}
	}
	if rec == nil {
		return re.BadRequestError("Enter a Clockify API key first.", nil)
	}
	if body.Workspace != nil {
		var ws []clockify.Workspace
		_ = rec.UnmarshalJSONField("workspaces", &ws)
		if !hasWorkspace(ws, *body.Workspace) {
			return re.BadRequestError("Unknown workspace.", nil)
		}
		rec.Set("workspace", *body.Workspace)
	}
	if body.Rate != nil {
		if *body.Rate < 0 {
			return re.BadRequestError("The hourly rate cannot be negative.", nil)
		}
		rec.Set("rate", *body.Rate)
	}
	if err := re.App.Save(rec); err != nil {
		return err
	}
	return re.JSON(http.StatusOK, settingsOf(rec))
}

func (s *Server) deleteClockify(re *core.RequestEvent) error {
	if rec, err := clockifyRecord(re.App, re.Auth.Id); err == nil {
		if err := re.App.Delete(rec); err != nil {
			return err
		}
	}
	return re.JSON(http.StatusOK, clockifySettings{})
}

// clockifyEntriesRoute fetches the user's own entries for inclusive dates
// {from, to}: one Clockify request per 1000 entries, usually exactly one.
func (s *Server) clockifyEntriesRoute(re *core.RequestEvent) error {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("Invalid request body.", err)
	}
	rec, err := clockifyRecord(re.App, re.Auth.Id)
	if err != nil {
		return re.BadRequestError("Clockify is not connected — add your API key under Settings.", nil)
	}
	tz := rec.GetString("timezone")
	start, end, err := clockify.DayRange(body.From, body.To, tz)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	c := newClockify(rec.GetString("api_key"))
	entries, err := c.TimeEntries(re.Request.Context(), rec.GetString("workspace"), rec.GetString("clockify_user"), start, end)
	if err != nil {
		return clockifyError(re, err)
	}
	out := clockifyEntries{From: body.From, To: body.To, Timezone: tz, Entries: []clockifyEntry{}}
	rates := map[int64]int{}
	for _, e := range entries {
		if e.Running() {
			out.Running++
			continue
		}
		ce := clockifyEntry{
			Start:       e.TimeInterval.Start.UTC().Format("2006-01-02T15:04:05Z"),
			Seconds:     e.Seconds(),
			Billable:    e.Billable,
			Project:     e.ProjectID,
			Description: e.Description,
			Tags:        []string{},
		}
		if e.Project != nil {
			ce.ProjectName, ce.Client = e.Project.Name, e.Project.ClientName
		}
		for _, t := range e.Tags {
			ce.Tags = append(ce.Tags, t.Name)
		}
		out.Entries = append(out.Entries, ce)
		if e.HourlyRate != nil && e.HourlyRate.Amount > 0 {
			rates[e.HourlyRate.Amount]++
			out.Currency = e.HourlyRate.Currency
		}
	}
	if len(rates) > 0 {
		amounts := make([]int64, 0, len(rates))
		for a := range rates {
			amounts = append(amounts, a)
		}
		sort.Slice(amounts, func(i, j int) bool {
			if rates[amounts[i]] != rates[amounts[j]] {
				return rates[amounts[i]] > rates[amounts[j]]
			}
			return amounts[i] > amounts[j]
		})
		r := float64(amounts[0]) / 100
		out.ClockifyRate = &r
	}
	return re.JSON(http.StatusOK, out)
}

func clockifyError(re *core.RequestEvent, err error) error {
	switch {
	case errors.Is(err, clockify.ErrUnauthorized):
		return re.BadRequestError("Clockify rejected the API key.", err)
	case errors.Is(err, clockify.ErrRateLimited):
		return re.TooManyRequestsError("Clockify's request limit is reached — try again later.", err)
	}
	re.App.Logger().Warn("clockify request failed", "err", err)
	return re.Error(http.StatusBadGateway, "Clockify request failed: "+err.Error(), err)
}

func hasWorkspace(ws []clockify.Workspace, id string) bool {
	for _, w := range ws {
		if w.ID == id {
			return true
		}
	}
	return false
}
