// Package migrations holds the schema history. Never edit an applied
// migration — later changes live in their own numbered files.
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// v1 schema (SPEC.md §4): templates, template_revisions, documents, renders,
// fragments, fonts. Every collection carries `user` and owner-only API rules.

const (
	ruleOwned  = "@request.auth.id != '' && user = @request.auth.id"
	ruleCreate = "@request.auth.id != '' && @request.body.user = @request.auth.id"
)

func ownerRules(c *core.Collection) {
	owned, create := ruleOwned, ruleCreate
	c.ListRule, c.ViewRule, c.UpdateRule, c.DeleteRule = &owned, &owned, &owned, &owned
	c.CreateRule = &create
}

// readOnlyRules: users may list/view (and optionally delete) but never create
// or update — such records are written by Go code only.
func readOnlyRules(c *core.Collection, allowDelete bool) {
	owned := ruleOwned
	c.ListRule, c.ViewRule = &owned, &owned
	c.CreateRule, c.UpdateRule, c.DeleteRule = nil, nil, nil
	if allowDelete {
		del := ruleOwned
		c.DeleteRule = &del
	}
}

func userField(usersId string) *core.RelationField {
	return &core.RelationField{Name: "user", Required: true, MaxSelect: 1, CollectionId: usersId, CascadeDelete: true}
}

func timestamps() []core.Field {
	return []core.Field{
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	}
}

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		// templates ------------------------------------------------------------
		templates := core.NewBaseCollection("templates")
		templates.Fields.Add(
			userField(users.Id),
			&core.TextField{Name: "name", Required: true, Max: 200},
			&core.TextField{Name: "html", Max: 500000},
			&core.TextField{Name: "css", Max: 200000},
			&core.JSONField{Name: "schema", MaxSize: 200000},
			&core.JSONField{Name: "theme", MaxSize: 20000},
			&core.JSONField{Name: "sample", MaxSize: 500000},
			&core.TextField{Name: "title_format", Max: 500},
			&core.SelectField{Name: "page", MaxSelect: 1, Values: []string{"A4", "Letter"}},
			&core.SelectField{Name: "locale", MaxSelect: 1, Values: []string{"de-AT", "de-DE", "en"}},
			&core.JSONField{Name: "sequences", MaxSize: 50000},
			&core.FileField{Name: "assets", MaxSelect: 20, MaxSize: 5 * 1024 * 1024, Protected: true},
			&core.NumberField{Name: "version", OnlyInt: true},
		)
		templates.Fields.Add(timestamps()...)
		templates.AddIndex("idx_templates_user_name", true, "user, name", "")
		ownerRules(templates)
		if err := app.Save(templates); err != nil {
			return err
		}

		// template_revisions (immutable snapshots) -------------------------------
		revisions := core.NewBaseCollection("template_revisions")
		revisions.Fields.Add(
			userField(users.Id),
			&core.RelationField{Name: "template", Required: true, MaxSelect: 1, CollectionId: templates.Id, CascadeDelete: true},
			&core.NumberField{Name: "version", Required: true, OnlyInt: true},
			&core.TextField{Name: "html", Max: 500000},
			&core.TextField{Name: "css", Max: 200000},
			&core.JSONField{Name: "schema", MaxSize: 200000},
			&core.JSONField{Name: "theme", MaxSize: 20000},
			&core.SelectField{Name: "page", MaxSelect: 1, Values: []string{"A4", "Letter"}},
			&core.SelectField{Name: "locale", MaxSelect: 1, Values: []string{"de-AT", "de-DE", "en"}},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		revisions.AddIndex("idx_revisions_template_version", true, "template, version", "")
		readOnlyRules(revisions, false)
		if err := app.Save(revisions); err != nil {
			return err
		}

		// documents ------------------------------------------------------------
		documents := core.NewBaseCollection("documents")
		documents.Fields.Add(
			userField(users.Id),
			&core.RelationField{Name: "template", Required: true, MaxSelect: 1, CollectionId: templates.Id},
			&core.TextField{Name: "title", Max: 500},
			&core.JSONField{Name: "data", MaxSize: 1000000},
		)
		documents.Fields.Add(timestamps()...)
		documents.AddIndex("idx_documents_user_template_updated", false, "user, template, updated", "")
		ownerRules(documents)
		if err := app.Save(documents); err != nil {
			return err
		}

		// renders (immutable, created by the render route) ---------------------
		renders := core.NewBaseCollection("renders")
		renders.Fields.Add(
			userField(users.Id),
			&core.RelationField{Name: "document", Required: true, MaxSelect: 1, CollectionId: documents.Id, CascadeDelete: true},
			&core.RelationField{Name: "revision", MaxSelect: 1, CollectionId: revisions.Id},
			&core.JSONField{Name: "data", MaxSize: 1000000},
			&core.TextField{Name: "html", Max: 5000000},
			&core.FileField{Name: "pdf", MaxSelect: 1, MaxSize: 50 * 1024 * 1024, MimeTypes: []string{"application/pdf"}, Protected: true},
			&core.TextField{Name: "title", Max: 500},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		renders.AddIndex("idx_renders_document_created", false, "document, created", "")
		readOnlyRules(renders, true)
		if err := app.Save(renders); err != nil {
			return err
		}

		// fragments --------------------------------------------------------------
		fragments := core.NewBaseCollection("fragments")
		fragments.Fields.Add(
			userField(users.Id),
			&core.TextField{Name: "name", Required: true, Max: 200},
			&core.TextField{Name: "kind", Required: true, Max: 100},
			&core.JSONField{Name: "data", MaxSize: 200000},
		)
		fragments.Fields.Add(timestamps()...)
		fragments.AddIndex("idx_fragments_user_kind", false, "user, kind", "")
		ownerRules(fragments)
		if err := app.Save(fragments); err != nil {
			return err
		}

		// fonts ------------------------------------------------------------------
		fonts := core.NewBaseCollection("fonts")
		fonts.Fields.Add(
			userField(users.Id),
			&core.TextField{Name: "family", Required: true, Max: 100},
			&core.NumberField{Name: "weight", Required: true, OnlyInt: true, Min: ptr(1.0), Max: ptr(1000.0)},
			&core.SelectField{Name: "style", MaxSelect: 1, Values: []string{"normal", "italic"}},
			&core.FileField{Name: "file", Required: true, MaxSelect: 1, MaxSize: 2 * 1024 * 1024,
				MimeTypes: []string{"font/woff2", "font/woff", "font/ttf", "font/otf", "font/sfnt", "application/font-woff", "application/font-woff2", "application/font-sfnt", "application/x-font-ttf", "application/x-font-opentype", "application/vnd.ms-opentype", "application/octet-stream"}},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		fonts.AddIndex("idx_fonts_user_family", false, "user, family", "")
		ownerRules(fonts)
		return app.Save(fonts)
	}, func(app core.App) error {
		for _, name := range []string{"renders", "fonts", "fragments", "documents", "template_revisions", "templates"} {
			if c, err := app.FindCollectionByNameOrId(name); err == nil {
				if err := app.Delete(c); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func ptr[T any](v T) *T { return &v }
