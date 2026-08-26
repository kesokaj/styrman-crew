// Package web embeds the built UI (dist/) into the styrman binary (ADR-006).
// Rebuild with: cd web && npm run build — the dist/ output is committed so
// `go build` needs no Node toolchain.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
