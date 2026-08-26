// paperparrot — make a template once, render it forever.
package main

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/cmd"

	"github.com/floholz/paperparrot/internal/cli"
)

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
	app.RootCmd.AddCommand(cli.NewRenderCommand())
	app.RootCmd.AddCommand(cmd.NewSuperuserCommand(app))
	serve := cmd.NewServeCommand(app, true)
	if f := serve.PersistentFlags().Lookup("http"); f != nil {
		_ = f.Value.Set(defaultAddr)
		f.DefValue = defaultAddr
	}
	app.RootCmd.AddCommand(serve)
	if err := app.Execute(); err != nil {
		log.Fatal(err)
	}
}
