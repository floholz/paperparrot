// paperparrot — make a template once, render it forever.
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/cmd"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/paperparrot/internal/api"
	"github.com/floholz/paperparrot/internal/cli"
	"github.com/floholz/paperparrot/internal/render"
	_ "github.com/floholz/paperparrot/migrations"
)

//go:embed all:ui/dist
var uiEmbed embed.FS

const defaultAddr = "127.0.0.1:8072"

// version is set at build time: -ldflags "-X main.version=v1.0.0"
var version = "dev"

func main() {
	// `render` needs no database: run it before the PocketBase app bootstraps.
	if len(os.Args) > 1 && os.Args[1] == "render" {
		c := cli.NewRenderCommand()
		c.SetArgs(os.Args[2:])
		if err := c.Execute(); err != nil {
			os.Exit(1)
		}
		return
	}

	app := pocketbase.New()
	renderer := render.NewChrome(render.FindChrome(), 2)
	srv := &api.Server{Renderer: renderer, Version: version}

	api.RegisterHooks(app)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := bootstrapSuperuser(app); err != nil {
			return err
		}
		if renderer.Available() {
			app.Logger().Info("PDF rendering via", "chrome", renderer.Path())
		} else {
			app.Logger().Warn("no Chromium found: PDF rendering disabled, preview still works (set PP_CHROME)")
		}
		srv.RegisterRoutes(e)
		ui, err := fs.Sub(uiEmbed, "ui/dist")
		if err != nil {
			return err
		}
		e.Router.GET("/{path...}", apis.Static(ui, true))
		return e.Next()
	})
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		renderer.Close()
		return e.Next()
	})

	// Same as app.Start(), but with our own default listen address.
	app.RootCmd.AddCommand(cli.NewRenderCommand())
	app.RootCmd.AddCommand(cmd.NewSuperuserCommand(app))
	serve := cmd.NewServeCommand(app, true)
	if f := serve.PersistentFlags().Lookup("http"); f != nil {
		_ = f.Value.Set(defaultAddr)
		f.DefValue = defaultAddr
		f.Usage = "TCP address to listen for the HTTP server"
	}
	app.RootCmd.AddCommand(serve)
	if err := app.Execute(); err != nil {
		log.Fatal(err)
	}
}

// bootstrapSuperuser creates the PocketBase admin (for /_/) from
// PP_ADMIN_EMAIL / PP_ADMIN_PASSWORD when no superuser exists yet.
func bootstrapSuperuser(app core.App) error {
	email, pass := os.Getenv("PP_ADMIN_EMAIL"), os.Getenv("PP_ADMIN_PASSWORD")
	if email == "" || pass == "" {
		return nil
	}
	n, err := app.CountRecords(core.CollectionNameSuperusers)
	if err != nil || n > 0 {
		return err
	}
	col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}
	u := core.NewRecord(col)
	u.SetEmail(email)
	u.SetPassword(pass)
	if err := app.Save(u); err != nil {
		return err
	}
	app.Logger().Info("created superuser from env", "email", email)
	return nil
}
