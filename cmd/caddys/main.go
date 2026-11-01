// Command caddys is the custom distribution's entry point.
// Keeping it separate from cmd/caddy lets upstream Caddy be merged unchanged.
package main

import (
	_ "time/tzdata"

	"github.com/caddyserver/caddy/v2"
	caddycmd "github.com/caddyserver/caddy/v2/cmd"
	"github.com/caddyserver/caddy/v2/cmd/caddys/sitecli"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	"github.com/spf13/cobra"
)

func init() {
	caddy.CustomBinaryName = "caddys"
	caddycmd.RegisterCommand(caddycmd.Command{
		Name: "site", Short: "Asistente seguro de sitios web", Usage: "<add|list|doctor> [opciones]",
		CobraFunc: func(cmd *cobra.Command) { sitecli.Configure(cmd) },
	})
}

func main() { caddycmd.Main() }
