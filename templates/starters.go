// Package templates ships the starter templates (one directory each:
// meta.json, body.html, style.css, schema.json, theme.json, sample.json).
package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed */meta.json */body.html */style.css */schema.json */theme.json */sample.json
var fs embed.FS

// Starter is one shipped template.
type Starter struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Page        string          `json:"page"`
	Locale      string          `json:"locale"`
	TitleFormat string          `json:"title_format"`
	Order       int             `json:"order"`
	HTML        string          `json:"-"`
	CSS         string          `json:"-"`
	Schema      json.RawMessage `json:"-"`
	Theme       json.RawMessage `json:"-"`
	Sample      json.RawMessage `json:"-"`
}

// All returns the starters sorted by their order.
func All() ([]Starter, error) {
	dirs, err := fs.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var out []Starter
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		s, err := load(d.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}

// Get returns a starter by id.
func Get(id string) (Starter, error) {
	all, err := All()
	if err != nil {
		return Starter{}, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Starter{}, fmt.Errorf("starter %q not found", id)
}

func load(dir string) (Starter, error) {
	var s Starter
	read := func(name string) ([]byte, error) { return fs.ReadFile(dir + "/" + name) }
	meta, err := read("meta.json")
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(meta, &s); err != nil {
		return s, fmt.Errorf("%s/meta.json: %w", dir, err)
	}
	s.ID = dir
	var b []byte
	if b, err = read("body.html"); err != nil {
		return s, err
	}
	s.HTML = string(b)
	if b, err = read("style.css"); err != nil {
		return s, err
	}
	s.CSS = string(b)
	if s.Schema, err = read("schema.json"); err != nil {
		return s, err
	}
	if s.Theme, err = read("theme.json"); err != nil {
		return s, err
	}
	if s.Sample, err = read("sample.json"); err != nil {
		return s, err
	}
	return s, nil
}
