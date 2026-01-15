package ib

import (
	"embed"
)

// Frontend contains the embedded web UI files
//
//go:embed all:frontend/dist
var Frontend embed.FS

// ClientBinaries contains the embedded client binaries for download
//
//go:embed all:dist/clients
var ClientBinaries embed.FS
