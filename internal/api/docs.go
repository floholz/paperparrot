// Package api holds the custom routes and the record hooks (SPEC.md §7).
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/paperparrot/internal/schema"
	"github.com/floholz/paperparrot/internal/theme"
	"github.com/floholz/paperparrot/internal/tmpl"
)

// snapshotFields are the template fields that make up a revision.
var snapshotFields = []string{"html", "css", "schema", "theme", "page", "locale"}

// docFromRecord builds a tmpl.Doc from a template or revision record (they
// share the snapshot fields). User fonts are attached for the theme families.
func docFromRecord(app core.App, rec *core.Record, userId string) (tmpl.Doc, error) {
	tokens, err := theme.ParseTokens([]byte(rec.GetString("theme")))
	if err != nil {
		return tmpl.Doc{}, err
	}
	fonts, err := userFonts(app, userId, tokens.Families())
	if err != nil {
		return tmpl.Doc{}, err
	}
	return tmpl.Doc{
		HTML:   rec.GetString("html"),
		CSS:    rec.GetString("css"),
		Theme:  tokens,
		Page:   rec.GetString("page"),
		Locale: rec.GetString("locale"),
		Fonts:  fonts,
	}, nil
}

// userFonts loads the uploaded faces of the given families.
func userFonts(app core.App, userId string, families []string) ([]theme.Font, error) {
	if userId == "" || len(families) == 0 {
		return nil, nil
	}
	recs, err := app.FindAllRecords("fonts", dbx.HashExp{"user": userId})
	if err != nil || len(recs) == 0 {
		return nil, err
	}
	want := map[string]bool{}
	for _, f := range families {
		want[strings.ToLower(strings.TrimSpace(f))] = true
	}
	var out []theme.Font
	for _, r := range recs {
		if !want[strings.ToLower(r.GetString("family"))] {
			continue
		}
		name := r.GetString("file")
		data, err := readFile(app, r, name)
		if err != nil {
			return nil, err
		}
		format := "woff2"
		switch ext := strings.ToLower(name[strings.LastIndex(name, ".")+1:]); ext {
		case "ttf":
			format = "truetype"
		case "otf":
			format = "opentype"
		case "woff":
			format = "woff"
		}
		style := r.GetString("style")
		if style == "" {
			style = "normal"
		}
		out = append(out, theme.Font{
			Family: r.GetString("family"),
			Weight: strconv.Itoa(r.GetInt("weight")),
			Style:  style,
			Format: format,
			Data:   data,
		})
	}
	return out, nil
}

// templateAssets reads all files of a template's `assets` field.
func templateAssets(app core.App, tpl *core.Record) (tmpl.Assets, error) {
	names := tpl.GetStringSlice("assets")
	if len(names) == 0 {
		return nil, nil
	}
	out := tmpl.Assets{}
	for _, n := range names {
		b, err := readFile(app, tpl, n)
		if err != nil {
			return nil, err
		}
		out[n] = b
	}
	return out, nil
}

func readFile(app core.App, rec *core.Record, name string) ([]byte, error) {
	fsys, err := app.NewFilesystem()
	if err != nil {
		return nil, err
	}
	defer fsys.Close()
	r, err := fsys.GetReader(rec.BaseFilesPath() + "/" + name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	defer r.Close()
	return io.ReadAll(r)
}

func jsonField(rec *core.Record, field string) map[string]any {
	var m map[string]any
	_ = rec.UnmarshalJSONField(field, &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func rawField(rec *core.Record, field string) json.RawMessage {
	s := rec.GetString(field)
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return json.RawMessage(s)
}

// deriveTitle executes the template's title_format with the document data.
func deriveTitle(tpl *core.Record, data map[string]any) string {
	if f := strings.TrimSpace(tpl.GetString("title_format")); f != "" {
		if t, err := tmpl.ExecuteText(f, data, tpl.GetString("locale")); err == nil && strings.TrimSpace(t) != "" {
			return t
		}
	}
	return tpl.GetString("name") + " " + time.Now().Format("2006-01-02")
}

// seqMu serialises counter increments: record create hooks do not run in a
// transaction, and the app is a single process.
var seqMu sync.Mutex

// assignSequences fills empty sequence fields from the template counters and
// writes the counters back (SPEC.md §6, "Sequence assignment").
func assignSequences(app core.App, tpl *core.Record, sc *schema.Schema, data map[string]any, now time.Time) (bool, error) {
	seqMu.Lock()
	defer seqMu.Unlock()
	// Re-read the template so the counters are current.
	fresh, err := app.FindRecordById("templates", tpl.Id)
	if err != nil {
		return false, err
	}
	tpl = fresh
	seqs := map[string]map[string]int{}
	_ = tpl.UnmarshalJSONField("sequences", &seqs)
	if seqs == nil {
		seqs = map[string]map[string]int{}
	}
	changed := false
	for _, f := range sc.SequenceFields() {
		if v, _ := data[f.Key].(string); strings.TrimSpace(v) != "" {
			continue
		}
		period := schema.SequencePeriod(f, now)
		if seqs[f.Key] == nil {
			seqs[f.Key] = map[string]int{}
		}
		seqs[f.Key][period]++
		data[f.Key] = schema.FormatSequence(f.Format, seqs[f.Key][period], now)
		changed = true
	}
	if !changed {
		return false, nil
	}
	tpl.Set("sequences", seqs)
	return true, app.Save(tpl)
}

// sanitizeFilename keeps a title usable as a file name.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		out = "document"
	}
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}
