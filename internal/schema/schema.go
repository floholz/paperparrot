// Package schema parses a template's field schema and validates document data
// against it. See SPEC.md §6.
package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Types accepted in a schema.
var Types = []string{"text", "textarea", "number", "money", "date", "bool", "select", "object", "list", "asset", "sequence"}

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Option of a select field.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

// Field definition. Pointer fields distinguish "unset" from zero.
type Field struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Label    string `json:"label,omitempty"`
	Help     string `json:"help,omitempty"`
	Required *bool  `json:"required,omitempty"`
	Default  any    `json:"default,omitempty"`

	Placeholder string   `json:"placeholder,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Step        *float64 `json:"step,omitempty"`
	Currency    string   `json:"currency,omitempty"`
	Options     []Option `json:"options,omitempty"`
	Fields      []Field  `json:"fields,omitempty"`
	Fragment    string   `json:"fragment,omitempty"`
	Accept      string   `json:"accept,omitempty"`
	Format      string   `json:"format,omitempty"`
	Reset       string   `json:"reset,omitempty"`
}

// IsRequired reports the effective required flag (default true).
func (f Field) IsRequired() bool { return f.Required == nil || *f.Required }

// DisplayLabel is the label or, when empty, the key.
func (f Field) DisplayLabel() string {
	if f.Label != "" {
		return f.Label
	}
	return f.Key
}

// Schema is the root: a flat list of fields; object and list nest.
type Schema struct {
	Fields []Field `json:"fields"`
}

// Error is a validation problem at a data path such as "items.2.amount".
type Error struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e Error) Error() string { return e.Path + ": " + e.Message }

// Errors is a list of validation errors that also satisfies error.
type Errors []Error

func (es Errors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

// Parse decodes and checks a schema. An empty input yields an empty schema.
func Parse(raw []byte) (*Schema, error) {
	s := &Schema{}
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return s, nil
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	if err := checkFields(s.Fields, "", 0); err != nil {
		return nil, err
	}
	return s, nil
}

func checkFields(fields []Field, path string, depth int) error {
	seen := map[string]bool{}
	for _, f := range fields {
		p := f.Key
		if path != "" {
			p = path + "." + f.Key
		}
		if !keyRe.MatchString(f.Key) {
			return fmt.Errorf("schema: invalid key %q at %q (want [a-z][a-z0-9_]*)", f.Key, path)
		}
		if seen[f.Key] {
			return fmt.Errorf("schema: duplicate key %q", p)
		}
		seen[f.Key] = true
		if !contains(Types, f.Type) {
			return fmt.Errorf("schema: field %q has unknown type %q", p, f.Type)
		}
		switch f.Type {
		case "object", "list":
			if len(f.Fields) == 0 {
				return fmt.Errorf("schema: field %q (%s) needs sub-fields", p, f.Type)
			}
			if depth >= 2 {
				return fmt.Errorf("schema: field %q nests too deep (max 3 levels)", p)
			}
			if err := checkFields(f.Fields, p, depth+1); err != nil {
				return err
			}
		case "select":
			if len(f.Options) == 0 {
				return fmt.Errorf("schema: select %q needs options", p)
			}
		case "sequence":
			if depth > 0 {
				return fmt.Errorf("schema: sequence %q must be top-level", p)
			}
			if f.Reset != "" && f.Reset != "never" && f.Reset != "year" {
				return fmt.Errorf("schema: sequence %q: reset must be \"never\" or \"year\"", p)
			}
		case "text":
			if f.Pattern != "" {
				if _, err := regexp.Compile(f.Pattern); err != nil {
					return fmt.Errorf("schema: field %q: bad pattern: %v", p, err)
				}
			}
		}
	}
	return nil
}

// Validate checks data against the schema. With strict=false only shape and
// types are checked (drafts may be incomplete); with strict=true required
// fields and list minimums are enforced too (render time).
func (s *Schema) Validate(data map[string]any, strict bool) Errors {
	var errs Errors
	validateFields(s.Fields, data, "", strict, &errs)
	return errs
}

func validateFields(fields []Field, data map[string]any, path string, strict bool, errs *Errors) {
	known := map[string]Field{}
	for _, f := range fields {
		known[f.Key] = f
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := known[k]; !ok {
			*errs = append(*errs, Error{join(path, k), "unknown field"})
		}
	}
	for _, f := range fields {
		p := join(path, f.Key)
		v, present := data[f.Key]
		if !present || isEmpty(v) {
			if strict && f.IsRequired() {
				*errs = append(*errs, Error{p, "required"})
			}
			continue
		}
		switch f.Type {
		case "text", "textarea", "asset", "sequence", "select":
			str, ok := v.(string)
			if !ok {
				*errs = append(*errs, Error{p, "must be a string"})
				continue
			}
			if f.Type == "text" && f.Pattern != "" {
				if re, err := regexp.Compile(f.Pattern); err == nil && !re.MatchString(str) {
					*errs = append(*errs, Error{p, "does not match pattern"})
				}
			}
			if f.Type == "select" {
				found := false
				for _, o := range f.Options {
					if o.Value == str {
						found = true
					}
				}
				if !found {
					*errs = append(*errs, Error{p, "not one of the options"})
				}
			}
		case "number", "money":
			n, ok := toFloat(v)
			if !ok {
				*errs = append(*errs, Error{p, "must be a number"})
				continue
			}
			if f.Min != nil && n < *f.Min {
				*errs = append(*errs, Error{p, fmt.Sprintf("must be ≥ %v", *f.Min)})
			}
			if f.Max != nil && n > *f.Max {
				*errs = append(*errs, Error{p, fmt.Sprintf("must be ≤ %v", *f.Max)})
			}
		case "date":
			str, ok := v.(string)
			if !ok || !dateRe.MatchString(str) {
				*errs = append(*errs, Error{p, "must be a date (YYYY-MM-DD)"})
				continue
			}
			if _, err := time.Parse("2006-01-02", str); err != nil {
				*errs = append(*errs, Error{p, "invalid date"})
			}
		case "bool":
			if _, ok := v.(bool); !ok {
				*errs = append(*errs, Error{p, "must be true or false"})
			}
		case "object":
			m, ok := v.(map[string]any)
			if !ok {
				*errs = append(*errs, Error{p, "must be an object"})
				continue
			}
			validateFields(f.Fields, m, p, strict, errs)
		case "list":
			arr, ok := v.([]any)
			if !ok {
				*errs = append(*errs, Error{p, "must be a list"})
				continue
			}
			if strict && f.Min != nil && float64(len(arr)) < *f.Min {
				*errs = append(*errs, Error{p, fmt.Sprintf("needs at least %v rows", *f.Min)})
			}
			if f.Max != nil && float64(len(arr)) > *f.Max {
				*errs = append(*errs, Error{p, fmt.Sprintf("at most %v rows", *f.Max)})
			}
			for i, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					*errs = append(*errs, Error{fmt.Sprintf("%s.%d", p, i), "must be an object"})
					continue
				}
				validateFields(f.Fields, m, fmt.Sprintf("%s.%d", p, i), strict, errs)
			}
		}
	}
}

// isEmpty treats nil, "" and empty lists as "not provided". Note that a list
// with zero rows is empty (so `min` applies), but false and 0 are values.
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// Empty builds a data object with every field present (empty strings, zero
// lists, nested objects) so that forms and templates have a stable shape.
// Defaults (including "today" for dates) are applied.
func (s *Schema) Empty(now time.Time) map[string]any {
	return emptyFields(s.Fields, now)
}

func emptyFields(fields []Field, now time.Time) map[string]any {
	m := map[string]any{}
	for _, f := range fields {
		m[f.Key] = f.EmptyValue(now)
	}
	return m
}

// EmptyValue is the initial value of a single field.
func (f Field) EmptyValue(now time.Time) any {
	switch f.Type {
	case "object":
		return emptyFields(f.Fields, now)
	case "list":
		rows := []any{}
		if f.Min != nil {
			for i := 0; i < int(*f.Min); i++ {
				rows = append(rows, emptyFields(f.Fields, now))
			}
		}
		return rows
	case "date":
		if d, ok := f.Default.(string); ok {
			if d == "today" {
				return now.Format("2006-01-02")
			}
			return d
		}
		return ""
	case "bool":
		if b, ok := f.Default.(bool); ok {
			return b
		}
		return false
	case "number", "money":
		if n, ok := toFloat(f.Default); ok {
			return n
		}
		return nil
	default:
		if d, ok := f.Default.(string); ok {
			return d
		}
		return ""
	}
}

// ApplyDefaults fills missing/empty top-level keys with their empty value
// (including "today" dates) and returns the same map for convenience.
func (s *Schema) ApplyDefaults(data map[string]any, now time.Time) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	for _, f := range s.Fields {
		if v, ok := data[f.Key]; !ok || isEmpty(v) {
			if f.Type == "sequence" {
				continue // assigned by the sequence hook
			}
			data[f.Key] = f.EmptyValue(now)
		}
	}
	return data
}

// ResetForDuplicate prepares data for "duplicate": sequence values are
// cleared so they get reassigned and dates with default "today" are reset.
func (s *Schema) ResetForDuplicate(data map[string]any, now time.Time) map[string]any {
	for _, f := range s.Fields {
		switch f.Type {
		case "sequence":
			data[f.Key] = ""
		case "date":
			if d, _ := f.Default.(string); d == "today" {
				data[f.Key] = now.Format("2006-01-02")
			}
		}
	}
	return data
}

// SequenceFields returns the top-level sequence fields.
func (s *Schema) SequenceFields() []Field {
	var out []Field
	for _, f := range s.Fields {
		if f.Type == "sequence" {
			out = append(out, f)
		}
	}
	return out
}

// SequencePeriod is the counter bucket for a sequence field: the year for
// reset "year", "all" otherwise.
func SequencePeriod(f Field, now time.Time) string {
	if f.Reset == "year" {
		return now.Format("2006")
	}
	return "all"
}

var seqTokenRe = regexp.MustCompile(`\{(n(?::\d+)?|yy|yyyy|mm)\}`)

// FormatSequence renders a sequence format such as "HN-{yy}-{n:3}".
// Tokens: {n}, {n:3} (zero-padded), {yy}, {yyyy}, {mm}. An empty format means "{n}".
func FormatSequence(format string, n int, now time.Time) string {
	if format == "" {
		format = "{n}"
	}
	return seqTokenRe.ReplaceAllStringFunc(format, func(tok string) string {
		switch inner := tok[1 : len(tok)-1]; {
		case inner == "yy":
			return now.Format("06")
		case inner == "yyyy":
			return now.Format("2006")
		case inner == "mm":
			return now.Format("01")
		case strings.HasPrefix(inner, "n:"):
			var w int
			fmt.Sscanf(inner[2:], "%d", &w)
			return fmt.Sprintf("%0*d", w, n)
		default:
			return fmt.Sprintf("%d", n)
		}
	})
}

// FindByKind returns the sub-schema of the first object field tagged with the
// given fragment kind (used by the fragments page to build a form).
func (s *Schema) FindByKind(kind string) *Field {
	for i := range s.Fields {
		if s.Fields[i].Type == "object" && s.Fields[i].Fragment == kind {
			return &s.Fields[i]
		}
	}
	return nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
