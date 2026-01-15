package main

// Note: Embedded files are set up in the internal/server package
// The frontendFS and clientBinaries are populated via server.SetEmbeddedFiles()
// during the build process. For development, these are empty embed.FS instances.
//
// In production builds, run `make build` which:
// 1. Builds the frontend to frontend/dist/
// 2. Builds client binaries to dist/clients/
// 3. Embeds these into the server binary
