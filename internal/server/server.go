package server

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/johann/ib/internal/config"
	"github.com/johann/ib/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Embed placeholders - these will be populated by the cmd/server build
var (
	frontendFS     embed.FS
	clientBinaries embed.FS
)

// SetEmbeddedFiles sets the embedded file systems (called from cmd/server)
func SetEmbeddedFiles(frontend, clients embed.FS) {
	frontendFS = frontend
	clientBinaries = clients
}

// Server represents the backup server
type Server struct {
	config      *config.ServerConfig
	storage     *storage.Storage
	router      *gin.Engine
	metricsPort int
	metrics     *Metrics
	title       string
}

// New creates a new server instance
func New(cfg *config.ServerConfig, metricsPort int, title string) (*Server, error) {
	store, err := storage.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	s := &Server{
		config:      cfg,
		storage:     store,
		router:      router,
		metricsPort: metricsPort,
		metrics:     NewMetrics(),
		title:       title,
	}

	s.setupRoutes()

	return s, nil
}

// Run starts the server
func (s *Server) Run() error {
	// Start metrics server if configured
	if s.metricsPort > 0 {
		go s.runMetricsServer()
	}

	// Start pruning job
	go s.runPruner()

	return s.router.Run(s.config.ListenAddr)
}

func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/api/health", s.handleHealth)
	s.router.GET("/api/config", s.handleConfig)

	// Public endpoints (no auth required)
	s.router.GET("/api/manifests", s.handleListManifests)
	s.router.GET("/api/manifests/:id", s.handleGetManifest)
	s.router.GET("/api/manifests/latest", s.handleGetLatestManifest)
	s.router.GET("/api/blocks/:cid", s.handleGetBlock)
	s.router.GET("/api/download/:manifest_id/*path", s.handleDownload)

	// CLI binary downloads
	s.router.GET("/cli/:os/:arch", s.handleCLIDownload)

	// Protected endpoints (auth required)
	protected := s.router.Group("/api")
	protected.Use(s.authMiddleware())
	{
		protected.POST("/manifests", s.handleCreateManifest)
		protected.DELETE("/manifests/:id", s.handleDeleteManifest)
		protected.POST("/blocks/:cid/exists", s.handleBlockExists)
		protected.POST("/blocks", s.handleUploadBlock)
	}

	// Static files (web UI)
	s.router.NoRoute(s.handleStaticFiles)
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Check for Bearer prefix
		const prefix = "Bearer "
		if len(token) > len(prefix) && token[:len(prefix)] == prefix {
			token = token[len(prefix):]
		}

		if token != s.config.Token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (s *Server) runMetricsServer() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.metricsPort),
		Handler: mux,
	}

	server.ListenAndServe()
}

func (s *Server) runPruner() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run once at startup
	s.prune()

	for range ticker.C {
		s.prune()
	}
}

func (s *Server) prune() {
	ctx := context.Background()
	cutoff := time.Now().AddDate(0, 0, -s.config.RetentionDays)

	if err := s.storage.PruneManifests(ctx, cutoff); err != nil {
		fmt.Printf("Pruning error: %v\n", err)
	}
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"title": s.title})
}
