package pinionrt

import "embed"

// Source holds the runtime sources, copied into each generated build.
//
//go:embed *.go
var Source embed.FS
