package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/volantvm/flint/pkg/config"
	"github.com/volantvm/flint/pkg/connectionpool"
	"github.com/volantvm/flint/pkg/imagerepository"
	"github.com/volantvm/flint/pkg/libvirtclient"
	"github.com/volantvm/flint/pkg/logger"
	"github.com/volantvm/flint/pkg/serverregistry"
	"golang.org/x/crypto/bcrypt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Server struct {
	router           *chi.Mux
	client           libvirtclient.ClientInterface
	assets           embed.FS
	apiKey           string
	passphraseHash   string
	rateLimiters     map[string]*rateLimiter
	rateLimiterMutex sync.RWMutex
	imageRepo        *imagerepository.ImageRepository
	sessions         map[string]time.Time // sessionID -> expiry time
	sessionsMu       sync.RWMutex
	consoleTokens    map[string]consoleToken
	consoleTokensMu  sync.Mutex
	serverRegistry   *serverregistry.Registry
	connectionPool   *connectionpool.Pool
}

type consoleToken struct {
	VMUUID string
	Kind   string
	Expiry time.Time
}

type rateLimiter struct {
	tokens     int
	lastRefill time.Time
}

func NewServer(client libvirtclient.ClientInterface, assets embed.FS) *Server {
	// Initialize image repository
	imageRepoPath := "/var/lib/flint/image-repository"
	imageRepo := imagerepository.NewImageRepository(imageRepoPath)

	s := &Server{
		router:         chi.NewRouter(),
		client:         client,
		assets:         assets,
		rateLimiters:   make(map[string]*rateLimiter),
		imageRepo:      imageRepo,
		sessions:       make(map[string]time.Time),
		consoleTokens:  make(map[string]consoleToken),
		connectionPool: connectionpool.NewPool(30*time.Minute, 5*time.Minute),
	}
	registry, err := serverregistry.NewRegistry("")
	if err != nil {
		logger.Fatal("Failed to initialize server registry", map[string]interface{}{"error": err.Error()})
	}
	s.serverRegistry = registry

	// Load or generate config
	s.loadOrGenerateConfig()

	logger.Info("Initializing Flint server", map[string]interface{}{
		"api_key_length": len(s.apiKey),
	})

	// Start session cleanup goroutine
	go s.cleanupExpiredSessions()

	s.router.Use(s.securityHeadersMiddleware)
	s.router.Use(s.rateLimitMiddleware)
	s.setupRoutes()
	return s
}

// loadOrGenerateConfig loads config from file or creates a new one with defaults
func (s *Server) loadOrGenerateConfig() {
	configDir := filepath.Join(os.Getenv("HOME"), ".flint")
	configPath := filepath.Join(configDir, "config.json")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		panic(fmt.Sprintf("failed to load security configuration: %v", err))
	}
	s.passphraseHash = cfg.Security.PassphraseHash
	if s.validateAPIKeyFormat(cfg.APIKey) {
		s.apiKey = cfg.APIKey
	} else {
		s.apiKey = s.generateAPIKey()
		cfg.APIKey = s.apiKey
	}
	if err := cfg.SaveConfig(configPath); err != nil {
		panic(fmt.Sprintf("failed to save security configuration: %v", err))
	}
	logger.Info("Loaded security configuration", map[string]interface{}{"config_path": configPath})
}

// generateAPIKey generates a secure API key
func (s *Server) generateAPIKey() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		logger.Error("Failed to generate secure API key", map[string]interface{}{
			"error": err.Error(),
		})
		// This is a critical security failure - exit rather than use weak key
		panic("Failed to generate cryptographically secure API key")
	}
	return hex.EncodeToString(bytes)
}

// createDefaultConfig creates a default configuration
func (s *Server) validateAPIKeyFormat(key string) bool {
	if len(key) != 64 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

// webAuthMiddleware validates web UI passphrase
func (s *Server) webAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for API endpoints (they use API key)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check for session cookie or prompt for passphrase
		cookie, err := r.Cookie("flint_session")
		if err == nil && s.isValidSession(cookie.Value) {
			// Valid session, continue
			next.ServeHTTP(w, r)
			return
		}

		// Check if this is a login POST request
		if r.Method == "POST" && r.URL.Path == "/login" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form data", http.StatusBadRequest)
				return
			}

			passphrase := r.FormValue("passphrase")
			if s.validatePassphrase(passphrase) {
				// Create new session
				sessionID, err := s.createSession()
				if err != nil {
					http.Error(w, "Failed to create session", http.StatusInternalServerError)
					return
				}

				// Set secure session cookie
				http.SetCookie(w, &http.Cookie{
					Name:     "flint_session",
					Value:    sessionID,
					Path:     "/",
					HttpOnly: true,
					Secure:   requestIsHTTPS(r),
					SameSite: http.SameSiteStrictMode,
					MaxAge:   60 * 60, // 1 hour
				})
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}

			// Invalid passphrase, show login form again
			s.showLoginForm(w, true)
			return
		}

		// Show login form
		s.showLoginForm(w, false)
	})
}

// showLoginForm displays the login form
func (s *Server) showLoginForm(w http.ResponseWriter, invalid bool) {
	w.Header().Set("Content-Type", "text/html")
	var errorMsg string
	if invalid {
		errorMsg = `<div style="color: red; margin-bottom: 10px;">Invalid passphrase</div>`
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Flint - Login</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; background: #f5f5f5; }
        .login { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); max-width: 400px; margin: 0 auto; }
        input[type="password"] { width: 100%%; padding: 10px; margin: 10px 0; border: 1px solid #ddd; border-radius: 4px; }
        button { background: #007bff; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; width: 100%%; }
        button:hover { background: #0056b3; }
    </style>
</head>
<body>
    <div class="login">
        <h2>🔐 Flint Web UI</h2>
        %s
        <form method="POST" action="/login">
            <input type="password" name="passphrase" placeholder="Enter passphrase" required>
            <button type="submit">Login</button>
        </form>
    </div>
</body>
</html>`, errorMsg)

	w.Write([]byte(html))
}

// validatePassphrase checks if the provided passphrase matches the hash
func (s *Server) validatePassphrase(passphrase string) bool {
	if passphrase == "" {
		return false
	}

	storedHash := s.getStoredPassphraseHash()

	// Check if it's an old SHA256 hash (64 hex chars) and migrate to bcrypt
	if len(storedHash) == 64 {
		// Legacy SHA256 hash - compare directly for backward compatibility
		hash := sha256.Sum256([]byte(passphrase))
		providedHash := hex.EncodeToString(hash[:])
		return providedHash == storedHash
	}

	// Use bcrypt comparison for new hashes
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(passphrase))
	return err == nil
}

// getStoredPassphraseHash returns the stored passphrase hash
func (s *Server) getStoredPassphraseHash() string {
	return s.passphraseHash
}

// authMiddleware validates API key from Authorization header OR session cookie
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First try API key authentication (for CLI/API usage)
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			// Expected format: "Bearer <api-key>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" && subtle.ConstantTimeCompare([]byte(parts[1]), []byte(s.apiKey)) == 1 {
				// Valid API key, continue
				ctx := context.WithValue(r.Context(), "api_key", parts[1])
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Fallback to session cookie authentication (for web app)
		cookie, err := r.Cookie("flint_session")
		if err == nil && s.isValidSession(cookie.Value) {
			if !sameOriginRequest(r) {
				http.Error(w, `{"error": "Cross-origin request rejected"}`, http.StatusForbidden)
				return
			}
			// Valid session from passphrase authentication
			next.ServeHTTP(w, r)
			return
		}

		// No valid authentication found
		http.Error(w, `{"error": "Authentication required. Use API key or login via web UI"}`, http.StatusUnauthorized)
	})
}

// rateLimitMiddleware implements rate limiting per client
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip rate limiting for health check
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Use client IP as the rate limiting key
		clientIP := s.getClientIP(r)

		capacity := 100
		perMinute := 100
		limiterKey := "api:" + clientIP
		if r.URL.Path == "/login" {
			capacity = 5
			perMinute = 5
			limiterKey = "login:" + clientIP
		}

		if !s.allowRequest(limiterKey, capacity, perMinute) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error": "Rate limit exceeded. Try again in 60 seconds."}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts the real client IP from the request
func (s *Server) getClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// allowRequest checks if a request should be allowed based on rate limiting
func (s *Server) allowRequest(key string, capacity, perMinute int) bool {
	s.rateLimiterMutex.Lock()
	defer s.rateLimiterMutex.Unlock()

	now := time.Now()
	limiter, exists := s.rateLimiters[key]

	if !exists {
		// New client - create limiter with 100 requests per minute
		s.rateLimiters[key] = &rateLimiter{
			tokens:     capacity - 1,
			lastRefill: now,
		}
		return true
	}

	// Refill tokens based on time passed
	timePassed := now.Sub(limiter.lastRefill)
	tokensToAdd := int(timePassed.Seconds() * float64(perMinute) / 60)

	if tokensToAdd > 0 {
		limiter.tokens = min(limiter.tokens+tokensToAdd, capacity)
		limiter.lastRefill = now
	}

	if limiter.tokens > 0 {
		limiter.tokens--
		return true
	}

	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Serve embedded static files via chi, without overriding API routes
func (s *Server) setupRoutes() {
	// Public API endpoint contains no host or VM details.
	s.router.Get("/api/health", s.handleHealthCheck())

	// Console WebSockets consume short-lived, one-time tokens.
	s.router.Get("/api/vms/{uuid}/serial-console/ws", s.handleVMSerialConsoleWS())

	// VNC console WebSocket endpoint (token-based auth, not middleware auth)
	s.router.Get("/api/vms/{uuid}/vnc/ws", s.handleVMVNCWebSocket())

	// Protected API routes with authentication
	s.router.Route("/api", func(r chi.Router) {
		r.Use(s.requestSizeLimitMiddleware(2 << 20))
		r.Use(s.authMiddleware)
		r.Get("/ssh-key/detect", s.handleDetectSSHKey())

		// Connection management endpoints
		r.Get("/connection/status", s.handleGetConnectionStatus())
		r.Post("/connection/test", s.handleTestConnection())
		r.Put("/connection/config", s.handleUpdateConnectionConfig())

		r.Get("/vms", s.handleGetVMs())
		r.Post("/vms", s.handleCreateVM())
		r.Post("/vms/from-template", s.handleCreateVMFromTemplate())
		r.Get("/vms/{uuid}", s.handleGetVMDetails())
		r.Delete("/vms/{uuid}", s.handleDeleteVM())
		r.Post("/vms/{uuid}/action", s.handleVMAction())
		r.Get("/vms/{uuid}/guest-agent/status", s.handleGetGuestAgentStatus())
		r.Post("/vms/{uuid}/guest-agent/install", s.handleInstallGuestAgent())
		r.Get("/vms/{uuid}/vnc", s.handleGetVMVNCInfo())
		r.Get("/vms/{uuid}/serial-console", s.handleGetVMSerialConsole())
		r.Get("/vms/{uuid}/console-stream", s.handleGetVMConsoleStream())
		r.Get("/vms/{uuid}/snapshots", s.handleGetVMSnapshots())
		r.Post("/vms/{uuid}/snapshots", s.handleCreateVMSnapshot())
		r.Delete("/vms/{uuid}/snapshots/{snapshotName}", s.handleDeleteVMSnapshot())
		r.Post("/vms/{uuid}/snapshots/{snapshotName}/revert", s.handleRevertToVMSnapshot())
		r.Get("/vm-templates", s.handleGetVMTemplates())
		r.Post("/vm-templates", s.handleCreateVMTemplate())
		r.Get("/vms/{uuid}/performance", s.handleGetVMPerformance())
		r.Post("/vms/{uuid}/attach-disk", s.handleAttachDiskToVM())
		r.Post("/vms/{uuid}/attach-network", s.handleAttachNetworkInterfaceToVM())
		r.Get("/host/status", s.handleGetHostStatus())
		r.Get("/host/resources", s.handleGetHostResources())
		r.Get("/storage-pools", s.handleGetStoragePools())
		r.Post("/storage-pools", s.handleCreateStoragePool())
		r.Get("/storage-pools/{poolName}/volumes", s.handleGetVolumes())
		r.Post("/storage-pools/{poolName}/volumes", s.handleCreateVolume())
		r.Put("/storage-pools/{poolName}/volumes/{volumeName}", s.handleUpdateVolume())
		r.Get("/networks", s.handleGetNetworks())
		r.Get("/system-interfaces", s.handleGetSystemInterfaces())
		r.Post("/networks", s.handleCreateNetwork())
		r.Post("/bridges", s.handleCreateBridge())
		r.Put("/networks/{networkName}", s.handleUpdateNetwork())
		r.Delete("/networks/{networkName}", s.handleDeleteNetwork())
		r.Delete("/storage-pools/{poolName}/volumes/{volumeName}", s.handleDeleteVolume())
		r.Get("/images", s.handleGetImages())
		r.Post("/images/import-from-path", s.handleImportImageFromPath())
		r.Post("/images/download", s.handleDownloadImage())
		r.Delete("/images/{imageId}", s.handleDeleteImage())

		// Image repository endpoints
		r.Get("/image-repository", s.handleGetRepositoryImages())
		r.Post("/image-repository/{imageId}/download", s.handleDownloadRepositoryImage())
		r.Get("/image-repository/{imageId}/status", s.handleGetDownloadStatus())
		r.Get("/activity", s.handleGetActivity())

		// Network filter / Firewall endpoints
		r.Get("/nwfilters", s.handleListNWFilters())
		r.Get("/nwfilters/{name}", s.handleGetNWFilter())
		r.Post("/nwfilters", s.handleCreateNWFilter())
		r.Put("/nwfilters/{name}", s.handleUpdateNWFilter())
		r.Delete("/nwfilters/{name}", s.handleDeleteNWFilter())
	})

	// Web UI routes with passphrase authentication
	// Create a sub-router for web UI with authentication middleware
	webRouter := chi.NewRouter()
	webRouter.Use(s.webAuthMiddleware)

	// Serve static files from embedded assets with middleware
	webRouter.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// Handle embedded assets from web/out/ and web/public/
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// First try web/out/ for built files
		embeddedPath := "web/out/" + path
		file, err := s.assets.Open(embeddedPath)
		if err != nil {
			// If not found in web/out/, try web/public/ for static assets
			publicPath := "web/public/" + path
			file, err = s.assets.Open(publicPath)
			if err != nil {
				// If file not found, try index.html for SPA routing
				if path != "index.html" {
					indexFile, indexErr := s.assets.Open("web/out/index.html")
					if indexErr == nil {
						defer indexFile.Close()
						w.Header().Set("Content-Type", "text/html")
						io.Copy(w, indexFile)
						return
					}
				}
				http.NotFound(w, r)
				return
			}
		}
		defer file.Close()

		// Set content type based on file extension
		if strings.HasSuffix(path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else if strings.HasSuffix(path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		} else if strings.HasSuffix(path, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		} else if strings.HasSuffix(path, ".png") {
			w.Header().Set("Content-Type", "image/png")
		} else if strings.HasSuffix(path, ".ico") {
			w.Header().Set("Content-Type", "image/x-icon")
		}

		io.Copy(w, file)
	})

	// Mount the web router to handle all non-API routes
	s.router.Mount("/", webRouter)
}

// Start starts the HTTP server with graceful shutdown
func (s *Server) Start(addr string) error {
	if addr == "" {
		addr = "127.0.0.1:5550"
	}

	logger.Info("Starting Flint server", map[string]interface{}{
		"address": addr,
	})

	// Create a server instance for graceful shutdown
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// Channel to listen for interrupt or terminate signals
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		logger.Info("Server is listening", map[string]interface{}{
			"address": addr,
		})

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}()

	// Wait for interrupt signal
	<-done
	logger.Info("Server is shutting down gracefully")

	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}
	s.connectionPool.CloseAll()

	logger.Info("Server shutdown complete")
	return nil
}

// validateAuthToken validates an authentication token

func (s *Server) validateAuthToken(token string) bool {
	// Validate token format and check against server's API key

	if token == "" {
		return false
	}

	// Check if token matches the server's API key
	if token != s.apiKey {
		return false
	}

	// Additional validation: check if token is a valid hex string of expected length
	if len(token) != 64 { // 32 bytes = 64 hex characters
		return false
	}

	// Check if it's valid hex
	for _, r := range token {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}

	return true
}

// GetAPIKey returns the server's API key (for CLI usage)
func (s *Server) GetAPIKey() string {
	return s.apiKey
}

// generateSessionID creates a cryptographically secure session ID
func (s *Server) generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// isValidSession checks if a session ID is valid and not expired
func (s *Server) isValidSession(sessionID string) bool {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()

	expiry, exists := s.sessions[sessionID]
	if !exists {
		return false
	}

	// Check if session has expired
	if time.Now().After(expiry) {
		// Clean up expired session
		go s.cleanupExpiredSession(sessionID)
		return false
	}

	return true
}

// createSession creates a new session and returns the session ID
func (s *Server) createSession() (string, error) {
	sessionID, err := s.generateSessionID()
	if err != nil {
		return "", err
	}

	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()

	// Initialize sessions map if needed
	if s.sessions == nil {
		s.sessions = make(map[string]time.Time)
	}

	// Session expires in 1 hour
	expiry := time.Now().Add(time.Hour)
	s.sessions[sessionID] = expiry

	return sessionID, nil
}

// cleanupExpiredSession removes an expired session
func (s *Server) cleanupExpiredSession(sessionID string) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	delete(s.sessions, sessionID)
}

// cleanupExpiredSessions periodically cleans up expired sessions
func (s *Server) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sessionsMu.Lock()
			now := time.Now()
			for sessionID, expiry := range s.sessions {
				if now.After(expiry) {
					delete(s.sessions, sessionID)
				}
			}
			s.sessionsMu.Unlock()
		}
	}
}
