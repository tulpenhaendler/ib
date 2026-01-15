package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/johann/ib/internal/backup"
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

	// Serialize and compress manifest
	data, err := json.Marshal(manifest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize manifest"})
		return
	}

	// Compress the manifest data
	compressed := compressData(data)

	if err := s.storage.SaveManifest(c.Request.Context(), &manifest, compressed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.metrics.manifestsTotal.Inc()

	c.JSON(http.StatusCreated, gin.H{"id": manifest.ID})
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

	// Parse format from extension
	format := "tar.gz"
	if strings.HasSuffix(manifestID, ".zip") {
		format = "zip"
		manifestID = strings.TrimSuffix(manifestID, ".zip")
	} else if strings.HasSuffix(manifestID, ".tar.gz") {
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
		s.streamZip(c, &manifest)
	} else {
		s.streamTarGz(c, &manifest)
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

func (s *Server) streamTarGz(c *gin.Context, manifest *backup.Manifest) {
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

		switch entry.Type {
		case backup.FileTypeDir:
			tw.WriteHeader(&tar.Header{
				Name:     entry.Path + "/",
				Mode:     int64(entry.Mode),
				Typeflag: tar.TypeDir,
			})

		case backup.FileTypeSymlink:
			tw.WriteHeader(&tar.Header{
				Name:     entry.Path,
				Mode:     int64(entry.Mode),
				Typeflag: tar.TypeSymlink,
				Linkname: entry.LinkTarget,
			})

		case backup.FileTypeFile:
			// Write header first with known size from manifest
			tw.WriteHeader(&tar.Header{
				Name: entry.Path,
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

func (s *Server) streamZip(c *gin.Context, manifest *backup.Manifest) {
	zw := zip.NewWriter(c.Writer)
	defer zw.Close()

	ctx := c.Request.Context()

	for _, entry := range manifest.Entries {
		select {
		case <-ctx.Done():
			return
		default:
		}

		switch entry.Type {
		case backup.FileTypeDir:
			zw.Create(entry.Path + "/")

		case backup.FileTypeSymlink:
			// Zip doesn't support symlinks well, create a small file with the target
			w, _ := zw.Create(entry.Path + ".symlink")
			w.Write([]byte(entry.LinkTarget))

		case backup.FileTypeFile:
			// Use CreateHeader with known size for streaming
			header := &zip.FileHeader{
				Name:   entry.Path,
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
