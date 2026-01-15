package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ipfs/go-cid"
	"github.com/johann/ib/internal/backup"
	"github.com/johann/ib/internal/ipfsnode"
)

func (s *Server) handleListManifests(c *gin.Context) {
	tags := extractTags(c)

	manifests, err := s.storage.ListManifests(c.Request.Context(), tags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, manifests)
}

func (s *Server) handleGetManifest(c *gin.Context) {
	id := c.Param("id")

	data, err := s.storage.GetManifest(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Decompress manifest data
	decompressed, err := backup.Decompress(data, int64(len(data)*10)) // Estimate
	if err != nil {
		// Might not be compressed
		decompressed = data
	}

	var manifest backup.Manifest
	if err := json.Unmarshal(decompressed, &manifest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse manifest"})
		return
	}

	c.JSON(http.StatusOK, manifest)
}

func (s *Server) handleGetLatestManifest(c *gin.Context) {
	tags := extractTags(c)

	data, err := s.storage.GetLatestManifest(c.Request.Context(), tags)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no manifests") {
			c.JSON(http.StatusNotFound, gin.H{"error": "no matching manifest found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Decompress manifest data
	decompressed, err := backup.Decompress(data, int64(len(data)*10))
	if err != nil {
		decompressed = data
	}

	var manifest backup.Manifest
	if err := json.Unmarshal(decompressed, &manifest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse manifest"})
		return
	}

	c.JSON(http.StatusOK, manifest)
}

func (s *Server) handleCreateManifest(c *gin.Context) {
	var manifest backup.Manifest
	if err := c.ShouldBindJSON(&manifest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid manifest JSON"})
		return
	}

	ctx := c.Request.Context()

	// Build IPFS DAG structure and collect node CIDs
	nodeCollector := ipfsnode.NewNodeCollector(s.storage)
	rootCID, err := ipfsnode.BuildManifestDAG(ctx, &manifest, nodeCollector)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to build DAG: %v", err)})
		return
	}

	// Update manifest with root CID (BuildManifestDAG already does this, but be explicit)
	manifest.RootCID = rootCID.String()

	// Serialize and compress manifest (after DAG building so it includes CIDs)
	data, err := json.Marshal(manifest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize manifest"})
		return
	}

	// Compress the manifest data
	compressed := compressData(data)

	// Save manifest with node references
	nodeCIDs := nodeCollector.NodeCIDs()
	if err := s.storage.SaveManifest(ctx, &manifest, compressed, nodeCIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.metrics.manifestsTotal.Inc()

	// Advertise root CID to DHT if IPFS is enabled
	if s.ipfsNode != nil && manifest.RootCID != "" {
		if rootCIDParsed, err := cid.Decode(manifest.RootCID); err == nil {
			s.ipfsNode.AddRootCID(rootCIDParsed)
			// Advertise in background to not block the response
			go func() {
				if err := s.ipfsNode.AdvertiseRoots(context.Background()); err != nil {
					fmt.Printf("Warning: failed to advertise root CID: %v\n", err)
				}
			}()
		}
	}

	c.JSON(http.StatusCreated, gin.H{"id": manifest.ID, "root_cid": manifest.RootCID})
}

func (s *Server) handleDeleteManifest(c *gin.Context) {
	id := c.Param("id")

	if err := s.storage.DeleteManifest(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.metrics.manifestsTotal.Dec()

	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (s *Server) handleGetBlock(c *gin.Context) {
	cid := c.Param("cid")

	data, err := s.storage.GetBlock(c.Request.Context(), cid)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "block not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.metrics.bandwidthDownload.Add(float64(len(data)))

	c.Data(http.StatusOK, "application/octet-stream", data)
}

func (s *Server) handleBlockExists(c *gin.Context) {
	cid := c.Param("cid")

	exists, err := s.storage.BlockExists(c.Request.Context(), cid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if exists {
		c.JSON(http.StatusOK, gin.H{"exists": true})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"exists": false})
	}
}

func (s *Server) handleUploadBlock(c *gin.Context) {
	cid := c.GetHeader("X-Block-CID")
	if cid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing X-Block-CID header"})
		return
	}

	originalSizeStr := c.GetHeader("X-Original-Size")
	originalSize, _ := strconv.ParseInt(originalSizeStr, 10, 64)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	if err := s.storage.SaveBlock(c.Request.Context(), cid, data, originalSize); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.metrics.blocksTotal.Inc()
	s.metrics.storageBytes.Add(float64(len(data)))
	s.metrics.bandwidthUpload.Add(float64(len(data)))

	c.JSON(http.StatusCreated, gin.H{"cid": cid})
}

func (s *Server) handleDownload(c *gin.Context) {
	manifestID := c.Param("manifest_id")

	// Parse format from extension (e.g., /api/download/abc123.tar.gz)
	format := "tar.gz"
	if strings.HasSuffix(manifestID, ".zip") {
		format = "zip"
		manifestID = strings.TrimSuffix(manifestID, ".zip")
	} else {
		manifestID = strings.TrimSuffix(manifestID, ".tar.gz")
	}

	// Get manifest
	data, err := s.storage.GetManifest(c.Request.Context(), manifestID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return
	}

	// Decompress manifest
	decompressed, err := backup.Decompress(data, int64(len(data)*10))
	if err != nil {
		decompressed = data
	}

	var manifest backup.Manifest
	if err := json.Unmarshal(decompressed, &manifest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse manifest"})
		return
	}

	// Set headers for download
	filename := manifestID
	if format == "zip" {
		filename += ".zip"
		c.Header("Content-Type", "application/zip")
	} else {
		filename += ".tar.gz"
		c.Header("Content-Type", "application/gzip")
	}
	c.Header("Content-Disposition", "attachment; filename="+filename)

	// Stream the archive
	if format == "zip" {
		s.streamZip(c, &manifest, "")
	} else {
		s.streamTarGz(c, &manifest, "")
	}
}

func (s *Server) handleDownloadFile(c *gin.Context) {
	manifestID := c.Param("manifest_id")
	filePath := c.Param("path")
	// Remove leading slash from wildcard param
	filePath = strings.TrimPrefix(filePath, "/")

	// Get manifest
	data, err := s.storage.GetManifest(c.Request.Context(), manifestID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return
	}

	// Decompress manifest
	decompressed, err := backup.Decompress(data, int64(len(data)*10))
	if err != nil {
		decompressed = data
	}

	var manifest backup.Manifest
	if err := json.Unmarshal(decompressed, &manifest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse manifest"})
		return
	}

	// Find the entry
	var targetEntry *backup.Entry
	for i := range manifest.Entries {
		if manifest.Entries[i].Path == filePath {
			targetEntry = &manifest.Entries[i]
			break
		}
	}

	if targetEntry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found in backup"})
		return
	}

	if targetEntry.Type != backup.FileTypeFile {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a file"})
		return
	}

	// Set headers
	filename := filepath.Base(filePath)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Length", strconv.FormatInt(targetEntry.Size, 10))

	// Stream the file blocks
	ctx := c.Request.Context()
	for _, cid := range targetEntry.Blocks {
		select {
		case <-ctx.Done():
			return
		default:
		}

		blockData, err := s.storage.GetBlock(ctx, cid)
		if err != nil {
			continue
		}

		decompressed, err := backup.Decompress(blockData, backup.ChunkSize)
		if err != nil {
			c.Writer.Write(blockData)
		} else {
			c.Writer.Write(decompressed)
		}
	}

	s.metrics.bandwidthDownload.Add(float64(targetEntry.Size))
}

func (s *Server) handleDownloadFolder(c *gin.Context) {
	manifestID := c.Param("manifest_id")
	folderPath := c.Param("path")
	// Remove leading slash from wildcard param
	folderPath = strings.TrimPrefix(folderPath, "/")

	// Parse format from extension
	format := "tar.gz"
	if strings.HasSuffix(folderPath, ".zip") {
		format = "zip"
		folderPath = strings.TrimSuffix(folderPath, ".zip")
	} else if strings.HasSuffix(folderPath, ".tar.gz") {
		folderPath = strings.TrimSuffix(folderPath, ".tar.gz")
	}

	// Get manifest
	data, err := s.storage.GetManifest(c.Request.Context(), manifestID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return
	}

	// Decompress manifest
	decompressed, err := backup.Decompress(data, int64(len(data)*10))
	if err != nil {
		decompressed = data
	}

	var manifest backup.Manifest
	if err := json.Unmarshal(decompressed, &manifest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse manifest"})
		return
	}

	// Filter entries to only include those in the folder
	var filteredEntries []backup.Entry
	folderPrefix := folderPath + "/"

	for _, entry := range manifest.Entries {
		// Include the folder itself or anything inside it
		if entry.Path == folderPath || strings.HasPrefix(entry.Path, folderPrefix) {
			filteredEntries = append(filteredEntries, entry)
		}
	}

	if len(filteredEntries) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found in backup"})
		return
	}

	// Create a filtered manifest
	filteredManifest := &backup.Manifest{
		ID:        manifest.ID,
		Tags:      manifest.Tags,
		CreatedAt: manifest.CreatedAt,
		RootPath:  manifest.RootPath,
		Entries:   filteredEntries,
	}

	// Set headers for download
	filename := filepath.Base(folderPath)
	if format == "zip" {
		filename += ".zip"
		c.Header("Content-Type", "application/zip")
	} else {
		filename += ".tar.gz"
		c.Header("Content-Type", "application/gzip")
	}
	c.Header("Content-Disposition", "attachment; filename="+filename)

	// Stream the archive with path prefix to strip
	if format == "zip" {
		s.streamZip(c, filteredManifest, folderPath)
	} else {
		s.streamTarGz(c, filteredManifest, folderPath)
	}
}

func (s *Server) handleCLIDownload(c *gin.Context) {
	osName := c.Param("os")
	arch := c.Param("arch")

	filename := "ib-" + osName + "-" + arch
	if osName == "windows" {
		filename += ".exe"
	}

	// Try to serve from embedded files
	data, err := clientBinaries.ReadFile("dist/clients/" + filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "binary not found for " + osName + "/" + arch})
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "application/octet-stream", data)
}

func (s *Server) handleStaticFiles(c *gin.Context) {
	path := c.Request.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	// Try to serve from embedded frontend files
	data, err := frontendFS.ReadFile("frontend/dist" + path)
	if err != nil {
		// Try index.html for SPA routing
		data, err = frontendFS.ReadFile("frontend/dist/index.html")
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
	}

	// Set content type based on extension
	contentType := "text/html"
	if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript"
	} else if strings.HasSuffix(path, ".css") {
		contentType = "text/css"
	} else if strings.HasSuffix(path, ".json") {
		contentType = "application/json"
	}

	c.Data(http.StatusOK, contentType, data)
}

func extractTags(c *gin.Context) map[string]string {
	tags := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if strings.HasPrefix(key, "tag.") {
			tagKey := strings.TrimPrefix(key, "tag.")
			if len(values) > 0 {
				tags[tagKey] = values[0]
			}
		}
	}
	return tags
}

func compressData(data []byte) []byte {
	// Use LZ4 compression
	compressed := make([]byte, len(data))
	n, err := backup.CompressBlock(data, compressed)
	if err != nil || n >= len(data) {
		return data
	}
	return compressed[:n]
}

func (s *Server) streamTarGz(c *gin.Context, manifest *backup.Manifest, stripPrefix string) {
	gw := gzip.NewWriter(c.Writer)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	ctx := c.Request.Context()

	for _, entry := range manifest.Entries {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Calculate relative path by stripping prefix
		entryPath := entry.Path
		if stripPrefix != "" {
			if entry.Path == stripPrefix {
				// This is the root folder itself, use base name
				entryPath = filepath.Base(entry.Path)
			} else {
				entryPath = strings.TrimPrefix(entry.Path, stripPrefix+"/")
			}
		}

		switch entry.Type {
		case backup.FileTypeDir:
			tw.WriteHeader(&tar.Header{
				Name:     entryPath + "/",
				Mode:     int64(entry.Mode),
				Typeflag: tar.TypeDir,
			})

		case backup.FileTypeSymlink:
			tw.WriteHeader(&tar.Header{
				Name:     entryPath,
				Mode:     int64(entry.Mode),
				Typeflag: tar.TypeSymlink,
				Linkname: entry.LinkTarget,
			})

		case backup.FileTypeFile:
			// Write header first with known size from manifest
			tw.WriteHeader(&tar.Header{
				Name: entryPath,
				Mode: int64(entry.Mode),
				Size: entry.Size,
			})

			// Stream blocks directly to tar writer
			for _, cid := range entry.Blocks {
				blockData, err := s.storage.GetBlock(ctx, cid)
				if err != nil {
					continue
				}
				// Decompress and write directly
				decompressed, err := backup.Decompress(blockData, backup.ChunkSize)
				if err != nil {
					tw.Write(blockData)
				} else {
					tw.Write(decompressed)
				}
			}
		}
	}
}

func (s *Server) streamZip(c *gin.Context, manifest *backup.Manifest, stripPrefix string) {
	zw := zip.NewWriter(c.Writer)
	defer zw.Close()

	ctx := c.Request.Context()

	for _, entry := range manifest.Entries {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Calculate relative path by stripping prefix
		entryPath := entry.Path
		if stripPrefix != "" {
			if entry.Path == stripPrefix {
				// This is the root folder itself, use base name
				entryPath = filepath.Base(entry.Path)
			} else {
				entryPath = strings.TrimPrefix(entry.Path, stripPrefix+"/")
			}
		}

		switch entry.Type {
		case backup.FileTypeDir:
			zw.Create(entryPath + "/")

		case backup.FileTypeSymlink:
			// Zip doesn't support symlinks well, create a small file with the target
			w, _ := zw.Create(entryPath + ".symlink")
			w.Write([]byte(entry.LinkTarget))

		case backup.FileTypeFile:
			// Use CreateHeader with known size for streaming
			header := &zip.FileHeader{
				Name:   entryPath,
				Method: zip.Deflate,
			}
			header.SetMode(0644)

			w, err := zw.CreateHeader(header)
			if err != nil {
				continue
			}

			// Stream blocks directly to zip writer
			for _, cid := range entry.Blocks {
				blockData, err := s.storage.GetBlock(ctx, cid)
				if err != nil {
					continue
				}
				// Decompress and write directly
				decompressed, err := backup.Decompress(blockData, backup.ChunkSize)
				if err != nil {
					w.Write(blockData)
				} else {
					w.Write(decompressed)
				}
			}
		}
	}
}
