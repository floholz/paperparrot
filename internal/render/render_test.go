package render_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/floholz/paperparrot/internal/render"
	"github.com/floholz/paperparrot/internal/theme"
	"github.com/floholz/paperparrot/internal/tmpl"
)

func TestPDF(t *testing.T) {
	path := render.FindChrome()
	if path == "" {
		t.Skip("no chromium found")
	}
	c := render.NewChrome(path, 2)
	defer c.Close()
	html, err := tmpl.Compose(tmpl.Doc{HTML: `<h1>Hello {{.name}}</h1><p class="box">{{money .x}}</p>`, Theme: theme.Tokens{"font-body": "Source Serif 4"}},
		map[string]any{"name": "Parrot", "x": 12.5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	start := time.Now()
	pdf, err := c.PDF(ctx, html)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first render %s, %d bytes", time.Since(start), len(pdf))
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("not a pdf")
	}
	start = time.Now()
	if _, err := c.PDF(ctx, html); err != nil {
		t.Fatal(err)
	}
	t.Logf("second render %s", time.Since(start))
}
