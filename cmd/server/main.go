package main

import (
	"os"

	ib "github.com/johann/ib"
	"github.com/johann/ib/internal/server"
)

func init() {
	// Set embedded files for the server
	server.SetEmbeddedFiles(ib.Frontend, ib.ClientBinaries)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
