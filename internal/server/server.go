package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ipfs/go-cid"
	"github.com/johann/ib/internal/backup"
	"github.com/johann/ib/internal/config"
	"github.com/johann/ib/internal/ipfsnode"
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
	ipfsNode    *ipfsnode.Node
	rateLimiter *RateLimiter
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
		rateLimiter: NewRateLimiter(15 * time.Second),
	}

	// Start IPFS node if enabled
	if cfg.IPFSEnabled {
		ipfsCfg := ipfsnode.DefaultConfig()
		if len(cfg.IPFSListenAddrs) > 0 {
			ipfsCfg.ListenAddrs = cfg.IPFSListenAddrs
		}
		if cfg.IPFSGatewayAddr != "" {
			ipfsCfg.GatewayAddr = cfg.IPFSGatewayAddr
		}
		// Set public IP for DHT announcements
		if cfg.IPFSPublicIP != "" {
			ipfsCfg.AnnounceAddrs = []string{
				fmt.Sprintf("/ip4/%s/tcp/4001", cfg.IPFSPublicIP),
				fmt.Sprintf("/ip4/%s/udp/4001/quic-v1", cfg.IPFSPublicIP),
			}
		}

		ipfsNode, err := ipfsnode.NewNode(context.Background(), store, ipfsCfg)
		if err != nil {
			store.Close()
			return nil, fmt.Errorf("failed to start IPFS node: %w", err)
		}
		s.ipfsNode = ipfsNode
		fmt.Printf("IPFS node started: %s\n", ipfsNode.PeerID())
		for _, addr := range ipfsNode.Addrs() {
			fmt.Printf("  Listening: %s/p2p/%s\n", addr, ipfsNode.PeerID())
		}
		if cfg.IPFSPublicIP != "" {
			fmt.Printf("  Announcing: /ip4/%s/tcp/4001/p2p/%s\n", cfg.IPFSPublicIP, ipfsNode.PeerID())
			fmt.Printf("  Announcing: /ip4/%s/udp/4001/quic-v1/p2p/%s\n", cfg.IPFSPublicIP, ipfsNode.PeerID())
		}
		if cfg.IPFSGatewayAddr != "" {
			fmt.Printf("  Gateway: http://localhost%s/ipfs/<cid>\n", cfg.IPFSGatewayAddr)
		}
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

	// Load existing root CIDs for IPFS if enabled
	if s.ipfsNode != nil {
		go s.loadExistingRootCIDs()
	}

	return s.router.Run(s.config.ListenAddr)
}

// loadExistingRootCIDs loads root CIDs from existing manifests and advertises them
func (s *Server) loadExistingRootCIDs() {
	ctx := context.Background()

	manifests, err := s.storage.ListManifests(ctx, nil)
	if err != nil {
		fmt.Printf("Warning: failed to list manifests for IPFS: %v\n", err)
		return
	}

	var loaded int
	for _, info := range manifests {
		data, err := s.storage.GetManifest(ctx, info.ID)
		if err != nil {
			continue
		}

		// Decompress
		decompressed, err := backup.Decompress(data, int64(len(data)*10))
		if err != nil {
			decompressed = data
		}

		var manifest struct {
			RootCID string `json:"root_cid"`
		}
		if err := json.Unmarshal(decompressed, &manifest); err != nil {
			continue
		}

		if manifest.RootCID != "" {
			if c, err := cid.Decode(manifest.RootCID); err == nil {
				s.ipfsNode.AddRootCID(c)
				loaded++
			}
		}
	}

	if loaded > 0 {
		fmt.Printf("Loaded %d root CIDs for IPFS\n", loaded)
		if err := s.ipfsNode.AdvertiseRoots(ctx); err != nil {
			fmt.Printf("Warning: failed to advertise root CIDs: %v\n", err)
		}
	}
}

// Close shuts down the server
func (s *Server) Close() error {
	if s.ipfsNode != nil {
		if err := s.ipfsNode.Close(); err != nil {
			fmt.Printf("Warning: failed to close IPFS node: %v\n", err)
		}
	}
	return s.storage.Close()
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

	// Download endpoints - specific routes first, then generic
	s.router.GET("/api/download/:manifest_id/file/*path", s.handleDownloadFile)
	s.router.GET("/api/download/:manifest_id/folder/*path", s.handleDownloadFolder)
	s.router.GET("/api/download/:manifest_id", s.handleDownload)

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
		clientIP := GetRealIP(c)

		// Check if IP is blocked due to previous failed attempts
		if s.rateLimiter.IsBlocked(clientIP) {
			LogFailedAuth(clientIP, "ip temporarily blocked", true)
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many failed attempts, try again later"})
			c.Abort()
			return
		}

		token := c.GetHeader("Authorization")
		if token == "" {
			LogFailedAuth(clientIP, "missing authorization header", false)
			s.rateLimiter.BlockIP(clientIP)
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
			LogFailedAuth(clientIP, "invalid token", false)
			s.rateLimiter.BlockIP(clientIP)
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
