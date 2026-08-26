package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/paperparrot/internal/schema"
	"github.com/floholz/paperparrot/templates"
)

// RegistrationOpen implements the PP_REGISTRATION policy:
//
//	"true"  → always open
//	"false" → always closed (superusers can still create users in /_/)
//	unset   → open until the first user exists, then closed
func RegistrationOpen(app core.App) (bool, error) {
	switch strings.ToLower(os.Getenv("PP_REGISTRATION")) {
	case "true", "1", "on", "yes":
		return true, nil
	case "false", "0", "off", "no":
		return false, nil
	}
	n, err := app.CountRecords("users")
	return n == 0, err
}

// RegisterHooks binds all record hooks (SPEC.md §7 "Hooks").
func RegisterHooks(app core.App) {
	// Registration policy.
	app.OnRecordCreateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		open, err := RegistrationOpen(e.App)
		if err != nil {
			return err
		}
		if !open {
			return e.ForbiddenError("Registration is closed on this instance.", nil)
		}
		return e.Next()
	})

	// Every new user starts with the shipped starter templates.
	app.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if err := SeedStarters(e.App, e.Record.Id); err != nil {
			e.App.Logger().Error("could not seed starter templates", "err", err)
		}
		return e.Next()
	})

	// Templates: validate schema/theme JSON, version + revision snapshot.
	app.OnRecordCreate("templates").BindFunc(func(e *core.RecordEvent) error {
		if err := checkTemplate(e.Record); err != nil {
			return err
		}
		e.Record.Set("version", 1)
		if e.Record.GetString("page") == "" {
			e.Record.Set("page", "A4")
		}
		if e.Record.GetString("locale") == "" {
			e.Record.Set("locale", "de-AT")
		}
		if err := e.Next(); err != nil {
			return err
		}
		return snapshotRevision(e.App, e.Record)
	})
	app.OnRecordUpdate("templates").BindFunc(func(e *core.RecordEvent) error {
		if err := checkTemplate(e.Record); err != nil {
			return err
		}
		orig := e.Record.Original()
		changed := false
		for _, f := range snapshotFields {
			if normJSON(e.Record.GetString(f)) != normJSON(orig.GetString(f)) {
				changed = true
				break
			}
		}
		if changed {
			e.Record.Set("version", orig.GetInt("version")+1)
		}
		if err := e.Next(); err != nil {
			return err
		}
		if changed {
			return snapshotRevision(e.App, e.Record)
		}
		return nil
	})
	// Refuse deleting a template that still has documents.
	app.OnRecordDeleteRequest("templates").BindFunc(func(e *core.RecordRequestEvent) error {
		n, err := e.App.CountRecords("documents", dbx.HashExp{"template": e.Record.Id})
		if err != nil {
			return err
		}
		if n > 0 {
			return e.BadRequestError(fmt.Sprintf("This template is used by %d document(s). Delete those first.", n), nil)
		}
		return e.Next()
	})

	// Documents: validate data shape, assign sequences, derive title.
	app.OnRecordCreate("documents").BindFunc(func(e *core.RecordEvent) error {
		tpl, sc, data, err := documentContext(e.App, e.Record)
		if err != nil {
			return err
		}
		now := time.Now()
		data = sc.ApplyDefaults(data, now)
		if errs := sc.Validate(data, false); len(errs) > 0 {
			return apis.NewBadRequestError("Data does not match the template schema: "+errs.Error(), errs)
		}
		if _, err := assignSequences(e.App, tpl, sc, data, now); err != nil {
			return err
		}
		e.Record.Set("data", data)
		if strings.TrimSpace(e.Record.GetString("title")) == "" {
			e.Record.Set("title", deriveTitle(tpl, data))
		}
		return e.Next()
	})
	app.OnRecordUpdate("documents").BindFunc(func(e *core.RecordEvent) error {
		tpl, sc, data, err := documentContext(e.App, e.Record)
		if err != nil {
			return err
		}
		if errs := sc.Validate(data, false); len(errs) > 0 {
			return apis.NewBadRequestError("Data does not match the template schema: "+errs.Error(), errs)
		}
		// Keep the title derived unless the user edited it by hand: an empty
		// title or one that still equals the derivation from the previous data
		// is re-derived from the new data.
		title := strings.TrimSpace(e.Record.GetString("title"))
		orig := e.Record.Original()
		if title == "" || title == deriveTitle(tpl, jsonField(orig, "data")) {
			e.Record.Set("title", deriveTitle(tpl, data))
		}
		return e.Next()
	})
}

// documentContext loads the document's template, parses its schema and data.
func documentContext(app core.App, doc *core.Record) (*core.Record, *schema.Schema, map[string]any, error) {
	tpl, err := app.FindRecordById("templates", doc.GetString("template"))
	if err != nil {
		return nil, nil, nil, apis.NewBadRequestError("Template not found.", err)
	}
	if tpl.GetString("user") != doc.GetString("user") {
		return nil, nil, nil, apis.NewBadRequestError("Template belongs to another user.", nil)
	}
	sc, err := schema.Parse(rawField(tpl, "schema"))
	if err != nil {
		return nil, nil, nil, apis.NewBadRequestError(err.Error(), err)
	}
	return tpl, sc, jsonField(doc, "data"), nil
}

func checkTemplate(rec *core.Record) error {
	if _, err := schema.Parse(rawField(rec, "schema")); err != nil {
		return apis.NewBadRequestError(err.Error(), err)
	}
	if strings.TrimSpace(rec.GetString("theme")) != "" {
		var m map[string]any
		if err := json.Unmarshal(rawField(rec, "theme"), &m); err != nil {
			return apis.NewBadRequestError("theme: must be a JSON object", err)
		}
	}
	return nil
}

// snapshotRevision stores an immutable copy of the rendered-relevant fields.
func snapshotRevision(app core.App, tpl *core.Record) error {
	col, err := app.FindCollectionByNameOrId("template_revisions")
	if err != nil {
		return err
	}
	rev := core.NewRecord(col)
	rev.Set("user", tpl.GetString("user"))
	rev.Set("template", tpl.Id)
	rev.Set("version", tpl.GetInt("version"))
	for _, f := range snapshotFields {
		rev.Set(f, tpl.Get(f))
	}
	return app.Save(rev)
}

// findRevision returns the template's current revision, creating the snapshot
// if it is missing.
func findRevision(app core.App, tpl *core.Record) (*core.Record, error) {
	rev, err := app.FindFirstRecordByFilter("template_revisions", "template = {:t} && version = {:v}",
		dbx.Params{"t": tpl.Id, "v": tpl.GetInt("version")})
	if err == nil {
		return rev, nil
	}
	if err := snapshotRevision(app, tpl); err != nil {
		return nil, err
	}
	return app.FindFirstRecordByFilter("template_revisions", "template = {:t} && version = {:v}",
		dbx.Params{"t": tpl.Id, "v": tpl.GetInt("version")})
}

// normJSON makes JSON comparisons whitespace-insensitive.
func normJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// SeedStarters copies the shipped starter templates into a user's account.
func SeedStarters(app core.App, userId string) error {
	all, err := templates.All()
	if err != nil {
		return err
	}
	for _, s := range all {
		if _, err := CreateFromStarter(app, userId, s); err != nil {
			return err
		}
	}
	return nil
}

// CreateFromStarter creates a template record for the user from a starter,
// de-duplicating the name if needed.
func CreateFromStarter(app core.App, userId string, s templates.Starter) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("templates")
	if err != nil {
		return nil, err
	}
	name := s.Name
	for i := 2; ; i++ {
		n, err := app.CountRecords("templates", dbx.HashExp{"user": userId, "name": name})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		name = fmt.Sprintf("%s (%d)", s.Name, i)
		if i > 50 {
			return nil, errors.New("too many templates with that name")
		}
	}
	rec := core.NewRecord(col)
	rec.Set("user", userId)
	rec.Set("name", name)
	rec.Set("html", s.HTML)
	rec.Set("css", s.CSS)
	rec.Set("schema", s.Schema)
	rec.Set("theme", s.Theme)
	rec.Set("sample", s.Sample)
	rec.Set("title_format", s.TitleFormat)
	rec.Set("page", s.Page)
	rec.Set("locale", s.Locale)
	if err := app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}
