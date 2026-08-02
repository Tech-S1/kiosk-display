package assets

import "embed"

//go:embed all:web-display
var WebDisplay embed.FS

//go:embed all:web-manager
var WebManager embed.FS

//go:embed all:extension
var Extension embed.FS
