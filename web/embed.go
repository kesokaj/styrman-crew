// Package web embeds the TanStack Start SPA-mode build (dist/client/) into
// the styrman binary. Rebuild with: cd web && npm run build — dist/client is
// committed so `go build` needs no Node; dist/server (the unused Node
// handler Start also emits) is gitignored.
package web

import "embed"

//go:embed all:dist/client
var Dist embed.FS
