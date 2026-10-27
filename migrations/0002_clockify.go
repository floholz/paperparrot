package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// clockify: one Clockify connection per user. No API rules at all — the key
// is only ever read and written by the /api/pp/clockify routes, so it never
// reaches the browser.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c := core.NewBaseCollection("clockify")
		c.Fields.Add(
			userField(users.Id),
			&core.TextField{Name: "api_key", Required: true, Max: 200, Hidden: true},
			&core.TextField{Name: "clockify_user", Max: 100},
			&core.TextField{Name: "clockify_name", Max: 200},
			&core.TextField{Name: "timezone", Max: 100},
			&core.JSONField{Name: "workspaces", MaxSize: 50000},
			&core.TextField{Name: "workspace", Max: 100},
			&core.NumberField{Name: "rate", Min: ptr(0.0)},
		)
		c.Fields.Add(timestamps()...)
		c.AddIndex("idx_clockify_user", true, "user", "")
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("clockify")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
