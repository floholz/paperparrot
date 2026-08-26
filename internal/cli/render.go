// Package cli holds the database-free `render` subcommand.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/floholz/paperparrot/internal/render"
	"github.com/floholz/paperparrot/internal/schema"
	"github.com/floholz/paperparrot/internal/theme"
	"github.com/floholz/paperparrot/internal/tmpl"
)

// NewRenderCommand builds `paperparrot render`: same compose + render code as
// the server, no database.
func NewRenderCommand() *cobra.Command {
	var (
		htmlPath, cssPath, schemaPath, themePath, dataPath, assetsDir, out, htmlOut string
		page, locale                                                                string
		strict                                                                      bool
	)
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render a template + data to PDF without the database",
		Example: `  paperparrot render -t body.html -c style.css -s schema.json --theme theme.json -d data.json -o out.pdf
  paperparrot render -t body.html -d data.json --html out.html   # composed HTML only, no Chromium needed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			read := func(p string) (string, error) {
				if p == "" {
					return "", nil
				}
				b, err := os.ReadFile(p)
				return string(b), err
			}
			body, err := read(htmlPath)
			if err != nil {
				return err
			}
			css, err := read(cssPath)
			if err != nil {
				return err
			}
			rawSchema, err := read(schemaPath)
			if err != nil {
				return err
			}
			rawTheme, err := read(themePath)
			if err != nil {
				return err
			}
			rawData, err := read(dataPath)
			if err != nil {
				return err
			}
			var data map[string]any
			if strings.TrimSpace(rawData) != "" {
				if err := json.Unmarshal([]byte(rawData), &data); err != nil {
					return fmt.Errorf("data: %w", err)
				}
			}
			if rawSchema != "" {
				s, err := schema.Parse([]byte(rawSchema))
				if err != nil {
					return err
				}
				data = s.ApplyDefaults(data, time.Now())
				if errs := s.Validate(data, strict); len(errs) > 0 {
					for _, e := range errs {
						fmt.Fprintln(os.Stderr, "data:", e.Error())
					}
					return fmt.Errorf("data does not match schema")
				}
			}
			tokens, err := theme.ParseTokens([]byte(rawTheme))
			if err != nil {
				return err
			}
			assets := tmpl.Assets{}
			if assetsDir != "" {
				entries, err := os.ReadDir(assetsDir)
				if err != nil {
					return err
				}
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					b, err := os.ReadFile(filepath.Join(assetsDir, e.Name()))
					if err != nil {
						return err
					}
					assets[e.Name()] = b
				}
			}
			doc := tmpl.Doc{HTML: body, CSS: css, Theme: tokens, Page: page, Locale: locale, Title: strings.TrimSuffix(filepath.Base(out), ".pdf")}
			html, err := tmpl.Compose(doc, data, assets)
			if err != nil {
				return err
			}
			if htmlOut != "" {
				if err := os.WriteFile(htmlOut, []byte(html), 0o644); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "wrote", htmlOut)
			}
			if out == "" {
				if htmlOut == "" {
					return fmt.Errorf("nothing to do: pass -o out.pdf and/or --html out.html")
				}
				return nil
			}
			chrome := render.NewChrome(render.FindChrome(), 1)
			defer chrome.Close()
			if !chrome.Available() {
				return render.ErrUnavailable
			}
			pdf, err := chrome.PDF(context.Background(), html)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, pdf, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", out, len(pdf))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&htmlPath, "template", "t", "", "body HTML template (required)")
	f.StringVarP(&cssPath, "css", "c", "", "template CSS")
	f.StringVarP(&schemaPath, "schema", "s", "", "schema JSON (validates data, applies defaults)")
	f.StringVar(&themePath, "theme", "", "theme tokens JSON")
	f.StringVarP(&dataPath, "data", "d", "", "data JSON")
	f.StringVarP(&assetsDir, "assets", "a", "", "directory with assets for {{asset}}")
	f.StringVarP(&out, "out", "o", "", "output PDF path")
	f.StringVar(&htmlOut, "html", "", "also/only write the composed HTML here")
	f.StringVar(&page, "page", "A4", "page size: A4 | Letter")
	f.StringVar(&locale, "locale", "de-AT", "locale: de-AT | de-DE | en")
	f.BoolVar(&strict, "strict", true, "enforce required fields")
	_ = cmd.MarkFlagRequired("template")
	return cmd
}
