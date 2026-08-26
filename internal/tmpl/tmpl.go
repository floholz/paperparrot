// Package tmpl executes a paperparrot template (html/template + FuncMap) and
// composes one self-contained HTML document. See SPEC.md §6a and §7.
package tmpl

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"html/template"
	"math"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"
	"unicode/utf8"

	"github.com/floholz/paperparrot/internal/theme"
)

// Doc is everything needed to render besides the data.
type Doc struct {
	HTML   string
	CSS    string
	Theme  theme.Tokens
	Page   string       // A4 | Letter
	Locale string       // de-AT | de-DE | en
	Fonts  []theme.Font // user-uploaded faces (built-ins are always available)
	Title  string       // <title> of the composed document
}

// Assets maps stored file names to their content (template assets).
type Assets map[string][]byte

// Compose executes the template with data and wraps it with styles and fonts
// into a single HTML string without external references.
func Compose(doc Doc, data map[string]any, assets Assets) (string, error) {
	if doc.Theme == nil {
		doc.Theme = theme.Tokens{}
	}
	body, err := Execute(doc, data, assets)
	if err != nil {
		return "", err
	}
	faces := theme.Select(doc.Theme.Families(), doc.Fonts)

	var b strings.Builder
	b.Grow(len(body) + 4096)
	fmt.Fprintf(&b, "<!doctype html>\n<html lang=%q>\n<head>\n<meta charset=\"utf-8\">\n<title>%s</title>\n<style>\n", langOf(doc.Locale), html.EscapeString(doc.Title))
	b.WriteString(theme.FontFaceCSS(faces))
	b.WriteString(theme.HeadCSS(doc.Theme, doc.Page))
	b.WriteString(theme.BaseCSS)
	if strings.TrimSpace(doc.CSS) != "" {
		b.WriteString("\n/* template */\n")
		b.WriteString(doc.CSS)
		b.WriteString("\n")
	}
	b.WriteString("</style>\n</head>\n<body>\n")
	b.WriteString(body)
	b.WriteString("\n</body>\n</html>\n")
	return b.String(), nil
}

// Execute runs only the body template (no wrapper).
func Execute(doc Doc, data map[string]any, assets Assets) (string, error) {
	t, err := template.New("body").Funcs(FuncMap(doc, assets)).Option("missingkey=zero").Parse(doc.HTML)
	if err != nil {
		return "", fmt.Errorf("template: %w", err)
	}
	var out bytes.Buffer
	if data == nil {
		data = map[string]any{}
	}
	if err := t.Execute(&out, data); err != nil {
		return "", fmt.Errorf("template: %w", err)
	}
	// missingkey=zero prints "<no value>" for absent keys of map[string]any —
	// a typo must render as empty, not fail (§6a).
	s := strings.ReplaceAll(out.String(), "&lt;no value&gt;", "")
	return strings.ReplaceAll(s, "<no value>", ""), nil
}

// ExecuteText runs a plain-text template (used for document titles).
func ExecuteText(format string, data map[string]any, locale string) (string, error) {
	t, err := texttemplate.New("title").Funcs(texttemplate.FuncMap(FuncMap(Doc{Locale: locale}, nil))).Option("missingkey=zero").Parse(format)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return "", err
	}
	s := strings.ReplaceAll(out.String(), "<no value>", "")
	return strings.Join(strings.Fields(s), " "), nil
}

var suffixRe = regexp.MustCompile(`^(.*)_[a-zA-Z0-9]{10}(\.[^.]+)?$`)

// FindAsset resolves a name against stored PocketBase file names, which carry
// a random suffix ("logo_Ab12Cd34Ef.png"): exact match first, then by
// original name.
func FindAsset(assets Assets, name string) ([]byte, string, bool) {
	if b, ok := assets[name]; ok {
		return b, name, true
	}
	for stored, b := range assets {
		if m := suffixRe.FindStringSubmatch(stored); m != nil && m[1]+m[2] == name {
			return b, stored, true
		}
	}
	return nil, "", false
}

// FuncMap builds the template functions for a document.
func FuncMap(doc Doc, assets Assets) template.FuncMap {
	loc := localeOf(doc.Locale)
	return template.FuncMap{
		"money": func(v any, currency ...string) string {
			cur := loc.currency
			if len(currency) > 0 && currency[0] != "" {
				cur = currency[0]
			}
			return loc.money(toFloat(v), cur)
		},
		"num": func(v any, decimals ...int) string {
			d := 2
			if len(decimals) > 0 {
				d = decimals[0]
			}
			return loc.number(toFloat(v), d)
		},
		"date": func(v any, layout ...string) string {
			t, ok := toTime(v)
			if !ok {
				return fmt.Sprint(v)
			}
			l := loc.dateLayout
			if len(layout) > 0 && layout[0] != "" {
				l = layout[0]
			}
			return loc.formatDate(t, l)
		},
		"sum": func(list any, key string) float64 {
			var total float64
			if rows, ok := list.([]any); ok {
				for _, r := range rows {
					if m, ok := r.(map[string]any); ok {
						total += toFloat(m[key])
					}
				}
			}
			return total
		},
		"add": func(a, b any) float64 { return toFloat(a) + toFloat(b) },
		"sub": func(a, b any) float64 { return toFloat(a) - toFloat(b) },
		"mul": func(a, b any) float64 { return toFloat(a) * toFloat(b) },
		"div": func(a, b any) float64 {
			if d := toFloat(b); d != 0 {
				return toFloat(a) / d
			}
			return 0
		},
		"nl2br": func(v any) template.HTML {
			s := html.EscapeString(fmt.Sprint(orEmpty(v)))
			s = strings.ReplaceAll(s, "\r\n", "\n")
			return template.HTML(strings.ReplaceAll(s, "\n", "<br>\n"))
		},
		"default": func(def any, v any) any {
			if isZero(v) {
				return def
			}
			return v
		},
		"upper": func(v any) string { return strings.ToUpper(fmt.Sprint(orEmpty(v))) },
		"lower": func(v any) string { return strings.ToLower(fmt.Sprint(orEmpty(v))) },
		"title": func(v any) string { return titleCase(fmt.Sprint(orEmpty(v))) },
		"asset": func(v any) template.URL {
			name := fmt.Sprint(orEmpty(v))
			b, stored, ok := FindAsset(assets, name)
			if !ok {
				return ""
			}
			m := mime.TypeByExtension(strings.ToLower(filepath.Ext(stored)))
			if m == "" {
				m = http.DetectContentType(b)
			}
			return template.URL("data:" + m + ";base64," + base64.StdEncoding.EncodeToString(b))
		},
		"theme": func(key string) string { return doc.Theme.Get(key) },
		"join": func(list any, sep string) string {
			var parts []string
			if xs, ok := list.([]any); ok {
				for _, x := range xs {
					parts = append(parts, fmt.Sprint(x))
				}
			}
			return strings.Join(parts, sep)
		},
		"seq": func(n any) []int {
			c := int(toFloat(n))
			out := make([]int, 0, max(c, 0))
			for i := 1; i <= c; i++ {
				out = append(out, i)
			}
			return out
		},
	}
}

// ---- locale ------------------------------------------------------------------

type locale struct {
	thousands, decimal string
	currency           string
	dateLayout         string
	symbolBefore       bool // "€ 1.200,00" vs "1.200,00 €"
	symbolSpace        bool
	months             []string
}

var deMonths = []string{"Jänner", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}
var deDEMonths = []string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}

func localeOf(name string) locale {
	switch strings.ToLower(name) {
	case "de-de":
		return locale{".", ",", "EUR", "02.01.2006", false, true, deDEMonths}
	case "en", "en-us", "en-gb":
		return locale{",", ".", "USD", "January 2, 2006", true, false, nil}
	default: // de-AT
		return locale{".", ",", "EUR", "02.01.2006", true, true, deMonths}
	}
}

func langOf(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "de") {
		return "de"
	}
	return "en"
}

// nbsp keeps symbol and amount on one line.
const nbsp = "\u00a0"

var symbols = map[string]string{"EUR": "€", "USD": "$", "GBP": "£", "CHF": "CHF", "JPY": "¥"}

func (l locale) money(v float64, cur string) string {
	sym, ok := symbols[strings.ToUpper(cur)]
	if !ok {
		sym = strings.ToUpper(cur)
	}
	n := l.number(v, 2)
	if l.symbolBefore {
		if l.symbolSpace || utf8.RuneCountInString(sym) > 1 {
			return sym + nbsp + n
		}
		return sym + n
	}
	return n + nbsp + sym
}

func (l locale) number(v float64, decimals int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(l.thousands)
		}
		b.WriteRune(c)
	}
	out := b.String()
	if frac != "" {
		out += l.decimal + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

func (l locale) formatDate(t time.Time, layout string) string {
	s := t.Format(layout)
	if l.months != nil && strings.Contains(layout, "January") {
		s = strings.Replace(s, t.Format("January"), l.months[t.Month()-1], 1)
	}
	return s
}

// ---- helpers -----------------------------------------------------------------

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(x), ",", "."), 64)
		return f
	}
	return 0
}

func toTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case string:
		x = strings.TrimSpace(x)
		for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05.000Z", "2006-01-02 15:04:05", "02.01.2006"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case bool:
		return !x
	case float64:
		return x == 0
	case int:
		return x == 0
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
