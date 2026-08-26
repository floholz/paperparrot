// Package theme turns theme tokens into CSS and ships the built-in fonts.
// See SPEC.md §6b.
package theme

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed base.css
var BaseCSS string

//go:embed fonts/*.woff2 fonts/manifest.json
var fontFS embed.FS

// Tokens are the theme values (token name → value), see Defaults for the keys.
type Tokens map[string]string

// Defaults for every token; unknown tokens are ignored.
var Defaults = Tokens{
	"font-body":      "Inter",
	"font-heading":   "",
	"font-size":      "11",
	"color-text":     "#111111",
	"color-heading":  "#111111",
	"color-accent":   "#111111",
	"color-muted":    "#666666",
	"color-block-bg": "#e8e8e8",
	"margin":         "2cm",
}

// Order of tokens as shown in the Theme form.
var Order = []string{"font-body", "font-heading", "font-size", "color-text", "color-heading", "color-accent", "color-muted", "color-block-bg", "margin"}

// Get returns a token with defaults applied (font-heading falls back to font-body).
func (t Tokens) Get(key string) string {
	if v := strings.TrimSpace(t[key]); v != "" {
		return v
	}
	if key == "font-heading" {
		return t.Get("font-body")
	}
	return Defaults[key]
}

// Families referenced by the theme (deduplicated).
func (t Tokens) Families() []string {
	body, head := t.Get("font-body"), t.Get("font-heading")
	if head == "" || head == body {
		return []string{body}
	}
	return []string{body, head}
}

// ParseTokens decodes the stored theme JSON (empty → no overrides).
func ParseTokens(raw []byte) (Tokens, error) {
	t := Tokens{}
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return t, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("theme: %w", err)
	}
	for k, v := range m {
		switch x := v.(type) {
		case string:
			t[k] = x
		case float64:
			t[k] = strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", x), "00"), ".")
		}
	}
	return t, nil
}

// PageSize returns CSS width/height for a page name.
func PageSize(page string) (w, h string) {
	if strings.EqualFold(page, "Letter") {
		return "8.5in", "11in"
	}
	return "210mm", "297mm"
}

// PageName normalises the page select value.
func PageName(page string) string {
	if strings.EqualFold(page, "Letter") {
		return "Letter"
	}
	return "A4"
}

// HeadCSS emits the custom properties, the @page rule and the on-screen
// "sheet" look (print media ignores the latter, so the PDF is unaffected).
func HeadCSS(t Tokens, page string) string {
	var b strings.Builder
	b.WriteString(":root{")
	for _, k := range Order {
		v := t.Get(k)
		if strings.HasPrefix(k, "font-") && k != "font-size" {
			v = fmt.Sprintf("%q", v)
		}
		if k == "font-size" {
			v = strings.TrimSuffix(v, "pt") + "pt"
		}
		fmt.Fprintf(&b, "--pp-%s:%s;", k, v)
	}
	b.WriteString("}\n")
	w, h := PageSize(page)
	fmt.Fprintf(&b, "@page{size:%s;margin:%s}\n", PageName(page), t.Get("margin"))
	fmt.Fprintf(&b, "@media screen{html{background:#9a9a9a;padding:1.5em 0}body{background:#fff;width:%s;min-height:%s;margin:0 auto;padding:%s;box-sizing:border-box;box-shadow:0 2px 12px rgba(0,0,0,.35)}}\n", w, h, t.Get("margin"))
	return b.String()
}

// Font is one face: a family at a weight/style with the binary data.
type Font struct {
	Family string `json:"family"`
	Weight string `json:"weight"` // "400" or a variable range "100 900"
	Style  string `json:"style"`  // normal | italic
	Subset string `json:"subset,omitempty"`
	Range  string `json:"range,omitempty"` // unicode-range, optional
	File   string `json:"file,omitempty"`
	Format string `json:"-"` // woff2 | truetype | opentype
	Data   []byte `json:"-"`
}

var (
	builtinOnce sync.Once
	builtin     []Font
)

// Builtin returns the embedded fonts (data loaded).
func Builtin() []Font {
	builtinOnce.Do(func() {
		raw, err := fontFS.ReadFile("fonts/manifest.json")
		if err != nil {
			panic("theme: fonts manifest: " + err.Error())
		}
		if err := json.Unmarshal(raw, &builtin); err != nil {
			panic("theme: fonts manifest: " + err.Error())
		}
		for i := range builtin {
			builtin[i].Format = "woff2"
			builtin[i].Data, err = fontFS.ReadFile("fonts/" + builtin[i].File)
			if err != nil {
				panic("theme: font file: " + err.Error())
			}
		}
	})
	return builtin
}

// Family summary for the font picker.
type Family struct {
	Family  string   `json:"family"`
	Weights []string `json:"weights"`
	Styles  []string `json:"styles"`
	Builtin bool     `json:"builtin"`
}

// Families groups fonts by family name, sorted.
func Families(fonts []Font, isBuiltin bool) []Family {
	byName := map[string]*Family{}
	for _, f := range fonts {
		fam := byName[f.Family]
		if fam == nil {
			fam = &Family{Family: f.Family, Builtin: isBuiltin}
			byName[f.Family] = fam
		}
		fam.Weights = appendUnique(fam.Weights, f.Weight)
		fam.Styles = appendUnique(fam.Styles, f.Style)
	}
	out := make([]Family, 0, len(byName))
	for _, f := range byName {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Family < out[j].Family })
	return out
}

// Select picks the faces (built-in + user-provided) for the given families.
func Select(families []string, user []Font) []Font {
	want := map[string]bool{}
	for _, f := range families {
		want[strings.ToLower(strings.TrimSpace(f))] = true
	}
	var out []Font
	for _, f := range Builtin() {
		if want[strings.ToLower(f.Family)] {
			out = append(out, f)
		}
	}
	for _, f := range user {
		if want[strings.ToLower(f.Family)] {
			out = append(out, f)
		}
	}
	return out
}

// FontFaceCSS inlines faces as @font-face rules with data: URIs.
func FontFaceCSS(fonts []Font) string {
	var b strings.Builder
	for _, f := range fonts {
		if len(f.Data) == 0 {
			continue
		}
		mime, format := "font/woff2", "woff2"
		switch f.Format {
		case "truetype", "ttf":
			mime, format = "font/ttf", "truetype"
		case "opentype", "otf":
			mime, format = "font/otf", "opentype"
		case "woff":
			mime, format = "font/woff", "woff"
		}
		style := f.Style
		if style == "" {
			style = "normal"
		}
		weight := f.Weight
		if weight == "" {
			weight = "400"
		}
		fmt.Fprintf(&b, "@font-face{font-family:%q;font-style:%s;font-weight:%s;font-display:block;src:url(data:%s;base64,%s) format(%q);",
			f.Family, style, weight, mime, base64.StdEncoding.EncodeToString(f.Data), format)
		if f.Range != "" {
			fmt.Fprintf(&b, "unicode-range:%s;", f.Range)
		}
		b.WriteString("}\n")
	}
	return b.String()
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
