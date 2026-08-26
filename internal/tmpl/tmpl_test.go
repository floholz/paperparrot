package tmpl_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/floholz/paperparrot/internal/schema"
	"github.com/floholz/paperparrot/internal/theme"
	"github.com/floholz/paperparrot/internal/tmpl"
	"github.com/floholz/paperparrot/templates"
)

func TestStartersCompose(t *testing.T) {
	starters, err := templates.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(starters) < 2 {
		t.Fatalf("expected starters, got %d", len(starters))
	}
	for _, s := range starters {
		sc, err := schema.Parse(s.Schema)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		var data map[string]any
		if err := json.Unmarshal(s.Sample, &data); err != nil {
			t.Fatalf("%s sample: %v", s.ID, err)
		}
		if errs := sc.Validate(data, true); len(errs) > 0 {
			t.Fatalf("%s sample invalid: %v", s.ID, errs)
		}
		tokens, err := theme.ParseTokens(s.Theme)
		if err != nil {
			t.Fatal(err)
		}
		html, err := tmpl.Compose(tmpl.Doc{HTML: s.HTML, CSS: s.CSS, Theme: tokens, Page: s.Page, Locale: s.Locale}, data, nil)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		for _, want := range []string{"@font-face", "@page{size:A4", "<!doctype html>", "Jane Doe"} {
			if !strings.Contains(html, want) {
				t.Errorf("%s: composed html lacks %q", s.ID, want)
			}
		}
		if strings.Contains(html, "no value") {
			t.Errorf("%s: composed html contains <no value>", s.ID)
		}
		title, err := tmpl.ExecuteText(s.TitleFormat, data, s.Locale)
		if err != nil || title == "" {
			t.Errorf("%s: title: %q %v", s.ID, title, err)
		}
	}
}

func TestFuncs(t *testing.T) {
	data := map[string]any{
		"amount": 1200.5, "date": "2026-05-31", "text": "a & b\nc",
		"items": []any{map[string]any{"amount": 1.0}, map[string]any{"amount": 2.25}},
	}
	cases := []struct{ locale, tpl, want string }{
		{"de-AT", `{{money .amount}}`, "€\u00a01.200,50"},
		{"de-DE", `{{money .amount}}`, "1.200,50\u00a0€"},
		{"en", `{{money .amount}}`, "$1,200.50"},
		{"en", `{{money .amount "EUR"}}`, "€1,200.50"},
		{"en", `{{money .amount "CHF"}}`, "CHF\u00a01,200.50"},
		{"de-AT", `{{date .date}}`, "31.05.2026"},
		{"de-AT", `{{date .date "2. January 2006"}}`, "31. Mai 2026"},
		{"en", `{{date .date}}`, "May 31, 2026"},
		{"de-AT", `{{nl2br .text}}`, "a &amp; b<br>\nc"},
		{"de-AT", `{{money (sum .items "amount")}}`, "€\u00a03,25"},
		{"de-AT", `{{default "—" .missing}}|{{.missing}}|{{with .nested}}{{.deep}}{{end}}`, "—||"},
		{"de-AT", `{{num (mul .amount 2) 1}}`, "2.401,0"},
		{"de-AT", `{{upper "abc"}} {{title "hello world"}}`, "ABC Hello World"},
	}
	for _, c := range cases {
		got, err := tmpl.Execute(tmpl.Doc{HTML: c.tpl, Locale: c.locale}, data, nil)
		if err != nil {
			t.Errorf("%s: %v", c.tpl, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s (%s): got %q want %q", c.tpl, c.locale, got, c.want)
		}
	}
}

func TestUnknownObjectErrors(t *testing.T) {
	// A missing top-level object cannot be dereferenced further; the preview
	// surfaces the error instead of silently rendering nothing.
	if _, err := tmpl.Execute(tmpl.Doc{HTML: `{{.nested.deep}}`}, map[string]any{}, nil); err == nil {
		t.Error("expected an error for .nested.deep on missing .nested")
	}
}

func TestAssetLookup(t *testing.T) {
	assets := tmpl.Assets{"logo_Ab12Cd34Ef.png": []byte("\x89PNG....")}
	out, err := tmpl.Execute(tmpl.Doc{HTML: `<img src="{{asset "logo.png"}}">|{{asset "nope.png"}}`}, nil, assets)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "data:image/png;base64,") || !strings.HasSuffix(out, "|") {
		t.Errorf("asset lookup: %q", out)
	}
}

func TestSchema(t *testing.T) {
	s, err := schema.Parse([]byte(`{"fields":[
		{"key":"n","type":"sequence","format":"HN-{yy}-{n:3}","reset":"year"},
		{"key":"d","type":"date","default":"today"},
		{"key":"items","type":"list","min":1,"fields":[{"key":"amount","type":"money"}]},
		{"key":"opt","type":"text","required":false}]}`))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	if got := schema.FormatSequence("HN-{yy}-{n:3}", 7, now); got != "HN-26-007" {
		t.Errorf("sequence: %q", got)
	}
	empty := s.Empty(now)
	if empty["d"] != "2026-08-26" || len(empty["items"].([]any)) != 1 {
		t.Errorf("empty: %v", empty)
	}
	errs := s.Validate(map[string]any{"n": "x", "d": "2026-13-01", "items": []any{}, "bogus": 1}, true)
	if len(errs) != 3 {
		t.Errorf("want 3 errors, got %v", errs)
	}
	if errs := s.Validate(map[string]any{"items": []any{}}, false); len(errs) != 0 {
		t.Errorf("draft validation should pass: %v", errs)
	}
}
