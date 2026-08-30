package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/volantvm/flint/pkg/core"
	"github.com/volantvm/flint/pkg/imagerepository"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var linuxInterfaceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func (s *Server) handleGetVMs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vms, err := s.client.GetVMSummaries()
		if err != nil {
			sendInternalError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vms)
	}
}

func (s *Server) handleGetVMDetails() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		if err := validateUUID(uuid); err != nil {
			sendError(w, err.Error(), http.StatusBadRequest)
			return
		}

		vm, err := s.client.GetVMDetails(uuid)
		if err != nil {
			// Check if it's a "domain not found" error
			if strings.Contains(err.Error(), "lookup domain") {
				http.Error(w, `{"error": "VM not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, `{"error": "Failed to get VM details"}`, http.StatusInternalServerError)
			}
			return
		}
		json.NewEncoder(w).Encode(vm)
	}
}

func (s *Server) handleGetVMSnapshots() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		snapshots, err := s.client.GetVMSnapshots(uuid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(snapshots)
	}
}

func (s *Server) handleCreateVMSnapshot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		var req core.CreateSnapshotRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		snapshot, err := s.client.CreateVMSnapshot(uuid, req)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshot)
	}
}

func (s *Server) handleDeleteVMSnapshot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		snapshotName := chi.URLParam(r, "snapshotName")

		err := s.client.DeleteVMSnapshot(uuid, snapshotName)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleGetRepositoryImages returns all available cloud images from the repository
func (s *Server) handleGetRepositoryImages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		images := s.imageRepo.GetImages()

		// Add download status to each image
		type ImageWithStatus struct {
			imagerepository.CloudImage
			Downloaded bool `json:"downloaded"`
		}

		var imagesWithStatus []ImageWithStatus
		for _, img := range images {
			imagesWithStatus = append(imagesWithStatus, ImageWithStatus{
				CloudImage: img,
				Downloaded: s.imageRepo.IsImageDownloaded(img.ID),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(imagesWithStatus)
	}
}

// handleDownloadRepositoryImage downloads a cloud image from the repository
func (s *Server) handleDownloadRepositoryImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		imageID := chi.URLParam(r, "imageId")
		if imageID == "" {
			http.Error(w, `{"error": "Image ID is required"}`, http.StatusBadRequest)
			return
		}

		// Allow re-download always (user can decide if they want to re-download)

		// Start download in background
		go func() {
			var lastProgress float64 = -1
			err := s.imageRepo.DownloadImage(imageID, func(downloaded, total int64) {
				// Only log progress every 10% to reduce spam
				progress := float64(downloaded) / float64(total) * 100
				if progress-lastProgress >= 10 || progress == 100 {
					fmt.Printf("Download progress for %s: %.1f%%\n", imageID, progress)
					lastProgress = progress
				}
			})

			if err != nil {
				fmt.Printf("Download failed for %s: %v\n", imageID, err)
			} else {
				fmt.Printf("Download completed for %s\n", imageID)

				// Import the downloaded image into the main image library
				downloadedPath := s.imageRepo.GetDownloadedImagePath(imageID)
				if downloadedPath != "" {
					fmt.Printf("Importing downloaded image %s from %s\n", imageID, downloadedPath)
					_, importErr := s.client.ImportImageFromPath(downloadedPath)
					if importErr != nil {
						fmt.Printf("Failed to import downloaded image %s: %v\n", imageID, importErr)
					} else {
						fmt.Printf("Successfully imported downloaded image %s\n", imageID)
					}
				}
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "started",
			"message": "Download started in background",
			"imageId": imageID,
		})
	}
}

// handleGetDownloadStatus returns the download status of an image
func (s *Server) handleGetDownloadStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		imageID := chi.URLParam(r, "imageId")
		if imageID == "" {
			http.Error(w, `{"error": "Image ID is required"}`, http.StatusBadRequest)
			return
		}

		downloaded := s.imageRepo.IsImageDownloaded(imageID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"imageId":    imageID,
			"downloaded": downloaded,
			"path":       s.imageRepo.GetDownloadedImagePath(imageID),
		})
	}
}

func (s *Server) handleRevertToVMSnapshot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		snapshotName := chi.URLParam(r, "snapshotName")

		err := s.client.RevertToVMSnapshot(uuid, snapshotName)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Revert action initiated."})
	}
}

func (s *Server) handleVMAction() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		var req struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}
		err := s.client.PerformVMAction(uuid, req.Action)
		if err != nil {
			if strings.Contains(err.Error(), "lookup domain") {
				http.Error(w, `{"error": "VM not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) handleDeleteVM() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")

		// Check query parameter for disk deletion (default: false for safety)
		deleteDisksStr := r.URL.Query().Get("deleteDisks")
		deleteDisks := false
		if deleteDisksStr == "true" {
			deleteDisks = true
		}

		err := s.client.DeleteVM(uuid, deleteDisks)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) handleGetHostStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := s.client.GetHostStatus()
		if err != nil {
			sendInternalError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

func (s *Server) handleGetHostResources() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resources, err := s.client.GetHostResources()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resources)
	}
}

func (s *Server) handleGetStoragePools() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pools, err := s.client.GetStoragePools()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(pools)
	}
}

func (s *Server) handleCreateStoragePool() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg core.PoolConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		// Validate input
		if cfg.Name == "" {
			http.Error(w, `{"error": "Pool name is required"}`, http.StatusBadRequest)
			return
		}
		if cfg.Path == "" {
			http.Error(w, `{"error": "Pool path is required"}`, http.StatusBadRequest)
			return
		}

		err := s.client.CreateStoragePool(cfg)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func (s *Server) handleGetNetworks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		networks, err := s.client.GetNetworks()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(networks)
	}
}

func (s *Server) handleGetSystemInterfaces() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		interfaces, err := s.client.GetSystemInterfaces()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(interfaces)
	}
}

func (s *Server) handleGetActivity() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		activity := s.client.GetActivity()
		json.NewEncoder(w).Encode(activity)
	}
}

func (s *Server) handleHealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check libvirt connectivity
		libvirtHealthy := true
		if err := s.checkLibvirtHealth(); err != nil {
			libvirtHealthy = false
		}

		_, hostErr := s.client.GetHostStatus()
		_, resourcesErr := s.client.GetHostResources()

		health := map[string]interface{}{
			"status":         s.calculateOverallHealth(libvirtHealthy, hostErr, resourcesErr),
			"timestamp":      time.Now().Unix(),
			"version":        "0.1.0",
			"uptime_seconds": time.Since(time.Now().Add(-time.Hour)).Seconds(), // Placeholder
			"checks": map[string]interface{}{
				"libvirt": map[string]interface{}{
					"healthy": libvirtHealthy,
				},
				"host_status": map[string]interface{}{
					"healthy": hostErr == nil,
					"error":   "",
				},
				"host_resources": map[string]interface{}{
					"healthy": resourcesErr == nil,
					"error":   "",
				},
			},
		}

		statusCode := http.StatusOK
		if health["status"] != "healthy" {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(health)
	}
}

// checkLibvirtHealth performs a basic health check on libvirt connectivity
func (s *Server) checkLibvirtHealth() error {
	// Try to get host status as a connectivity test
	_, err := s.client.GetHostStatus()
	return err
}

// calculateOverallHealth determines the overall health status
func (s *Server) calculateOverallHealth(libvirtHealthy bool, hostErr, resourcesErr error) string {
	if !libvirtHealthy || hostErr != nil || resourcesErr != nil {
		return "unhealthy"
	}
	return "healthy"
}

func (s *Server) handleGetImages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		images, err := s.client.GetImages()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(images)
	}
}

func (s *Server) handleImportImageFromPath() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		// Validate file path
		if err := validateFilePath(req.Path); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		image, err := s.client.ImportImageFromPath(req.Path)
		if err != nil {
			http.Error(w, `{"error": "Failed to import image"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(image)
	}
}

func (s *Server) handleDownloadImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL  string `json:"url"`
			Name string `json:"name,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		// Validate URL
		if req.URL == "" {
			http.Error(w, `{"error": "URL is required"}`, http.StatusBadRequest)
			return
		}

		// Parse URL to validate it
		parsedURL, err := url.Parse(req.URL)
		if err != nil {
			http.Error(w, `{"error": "Invalid URL format"}`, http.StatusBadRequest)
			return
		}

		if err := validateDownloadURL(parsedURL); err != nil {
			http.Error(w, `{"error": "URL destination is not allowed"}`, http.StatusBadRequest)
			return
		}

		// Generate filename if not provided
		filename := req.Name
		if filename == "" {
			// Extract filename from URL
			urlPath := parsedURL.Path
			parts := strings.Split(urlPath, "/")
			if len(parts) > 0 {
				filename = parts[len(parts)-1]
			}
			// If still empty, generate a default name
			if filename == "" {
				filename = "downloaded-image-" + time.Now().Format("20060102-150405")
			}
		}

		filename = filepath.Base(filename)
		if filename == "." || filename == string(filepath.Separator) {
			filename = "downloaded-image"
		}

		// Create temporary file without including untrusted input in the path pattern.
		tempFile, err := os.CreateTemp("", "flint-download-*")
		if err != nil {
			http.Error(w, `{"error": "Failed to create temporary file: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer os.Remove(tempFile.Name()) // Clean up temp file
		defer tempFile.Close()

		// Download the file
		resp, err := safeHTTPClient().Get(req.URL)
		if err != nil {
			http.Error(w, `{"error": "Failed to download file: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Check if the download was successful
		if resp.StatusCode != http.StatusOK {
			http.Error(w, `{"error": "Failed to download file: HTTP `+strconv.Itoa(resp.StatusCode)+`"}`, http.StatusInternalServerError)
			return
		}
		if resp.ContentLength > maxDownloadSize {
			http.Error(w, `{"error": "Download exceeds the 10 GiB limit"}`, http.StatusRequestEntityTooLarge)
			return
		}

		// Limit streaming responses even when Content-Length is absent or false.
		written, err := io.Copy(tempFile, io.LimitReader(resp.Body, maxDownloadSize+1))
		if err != nil {
			http.Error(w, `{"error": "Failed to save downloaded file: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		if written > maxDownloadSize {
			http.Error(w, `{"error": "Download exceeds the 10 GiB limit"}`, http.StatusRequestEntityTooLarge)
			return
		}

		// Close the temp file so we can import it
		tempFile.Close()

		// Import the downloaded file into the managed image library
		image, err := s.client.ImportImageFromPath(tempFile.Name())
		if err != nil {
			http.Error(w, `{"error": "Failed to import downloaded image: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		// Return the imported image info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(image)
	}
}

func (s *Server) handleGetISOs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isos, err := s.client.GetISOs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(isos)
	}
}

func (s *Server) handleGetTemplates() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		templates, err := s.client.GetTemplates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(templates)
	}
}

func (s *Server) handleGetVMPerformance() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		perf, err := s.client.GetVMPerformance(uuid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(perf)
	}
}

func (s *Server) handleGetVMSerialConsole() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		if err := validateUUID(uuid); err != nil {
			sendError(w, err.Error(), http.StatusBadRequest)
			return
		}
		token, err := s.issueConsoleToken(uuid, "serial")
		if err != nil {
			sendInternalError(w, err)
			return
		}

		response := map[string]string{
			"websocket_path": fmt.Sprintf("/api/vms/%s/serial-console/ws", uuid),
			"token":          token,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func (s *Server) handleGetGuestAgentStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vmUUID := chi.URLParam(r, "uuid")
		if vmUUID == "" {
			http.Error(w, `{"error": "VM UUID is required"}`, http.StatusBadRequest)
			return
		}

		available, err := s.client.CheckGuestAgentStatus(vmUUID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to check guest agent status: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"available": available,
			"vm_uuid":   vmUUID,
		})
	}
}

func (s *Server) handleInstallGuestAgent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vmUUID := chi.URLParam(r, "uuid")
		if vmUUID == "" {
			http.Error(w, `{"error": "VM UUID is required"}`, http.StatusBadRequest)
			return
		}

		err := s.client.InstallGuestAgent(vmUUID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to install guest agent: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Guest agent installation attempted. Please wait a few moments and check status.",
		})
	}
}

func (s *Server) handleVMSerialConsoleWS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")

		// Authenticate using token from query parameters
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Authentication token required", http.StatusUnauthorized)
			return
		}

		if !s.consumeConsoleToken(token, uuid, "serial") {
			http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
			return
		}

		// Upgrade HTTP connection to WebSocket
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return sameWebSocketOrigin(r, s.trustedOrigins)
			},
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "Failed to upgrade to WebSocket", http.StatusBadRequest)
			return
		}
		defer conn.Close()

		// Get the PTY path for the VM
		ptyPath, err := s.client.GetVMSerialConsolePath(uuid)
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
			return
		}
		cleanPTYPath := filepath.Clean(ptyPath)
		if !strings.HasPrefix(cleanPTYPath, "/dev/pts/") {
			conn.WriteMessage(websocket.TextMessage, []byte("Error: invalid PTY path"))
			return
		}
		info, err := os.Stat(cleanPTYPath)
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			conn.WriteMessage(websocket.TextMessage, []byte("Error: invalid PTY device"))
			return
		}

		// Open the PTY device
		ptyFile, err := os.OpenFile(cleanPTYPath, os.O_RDWR, 0)
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte("Error opening PTY: "+err.Error()))
			return
		}
		defer ptyFile.Close()

		// Send connection confirmation
		conn.WriteMessage(websocket.TextMessage, []byte("Serial console connected\r\n"))

		// Set up bidirectional data flow with proper error handling
		var wg sync.WaitGroup
		wg.Add(2)

		// Channel to signal when either goroutine exits
		done := make(chan struct{})

		// WebSocket -> PTY
		go func() {
			defer wg.Done()
			defer close(done)

			for {
				select {
				case <-done:
					return
				default:
					messageType, data, err := conn.ReadMessage()
					if err != nil {
						// Connection closed or error
						return
					}

					// Only process text messages
					if messageType == websocket.TextMessage {
						_, err = ptyFile.Write(data)
						if err != nil {
							// PTY write error
							conn.WriteMessage(websocket.TextMessage, []byte("Error writing to PTY: "+err.Error()))
							return
						}
					}
				}
			}
		}()

		// PTY -> WebSocket
		go func() {
			defer wg.Done()

			reader := bufio.NewReader(ptyFile)
			buffer := make([]byte, 1024)

			for {
				select {
				case <-done:
					return
				default:
					n, err := reader.Read(buffer)
					if err != nil {
						if err != io.EOF {
							conn.WriteMessage(websocket.TextMessage, []byte("Error reading from PTY: "+err.Error()))
						}
						return
					}

					if n > 0 {
						err = conn.WriteMessage(websocket.TextMessage, buffer[:n])
						if err != nil {
							// WebSocket write error
							return
						}
					}
				}
			}
		}()

		wg.Wait()
	}
}

// validateVMCreationConfig validates VM creation configuration
func validateVMCreationConfig(cfg *core.VMCreationConfig) error {
	// Validate VM name
	if cfg.Name == "" {
		return fmt.Errorf("VM name is required")
	}
	if len(cfg.Name) > 64 {
		return fmt.Errorf("VM name must be 64 characters or less")
	}
	// Allow alphanumeric, hyphens, and underscores
	nameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !nameRegex.MatchString(cfg.Name) {
		return fmt.Errorf("VM name can only contain letters, numbers, hyphens, and underscores")
	}

	// Validate memory
	if cfg.MemoryMB == 0 {
		return fmt.Errorf("memory must be greater than 0 MB")
	}
	if cfg.MemoryMB > 524288 { // 512 GB limit
		return fmt.Errorf("memory cannot exceed 512 GB")
	}

	// Validate vCPUs
	if cfg.VCPUs <= 0 {
		return fmt.Errorf("vCPUs must be greater than 0")
	}
	if cfg.VCPUs > 128 {
		return fmt.Errorf("vCPUs cannot exceed 128")
	}

	// Validate image name (optional for PXE boot)
	if cfg.ImageName == "" && cfg.ImageType != "pxe" {
		return fmt.Errorf("image name is required")
	}
	if len(cfg.ImageName) > 255 {
		return fmt.Errorf("image name must be 255 characters or less")
	}

	// Validate image type
	if cfg.ImageType != "" && cfg.ImageType != "iso" && cfg.ImageType != "template" && cfg.ImageType != "pxe" {
		return fmt.Errorf("image type must be 'iso', 'template', or 'pxe'")
	}

	// Validate network name
	if cfg.NetworkName != "" && len(cfg.NetworkName) > 50 {
		return fmt.Errorf("network name must be 50 characters or less")
	}

	// Validate disk pool
	if cfg.DiskPool != "" && len(cfg.DiskPool) > 50 {
		return fmt.Errorf("disk pool name must be 50 characters or less")
	}

	// Validate disk size
	if cfg.DiskSizeGB > 10000 { // 10 TB limit
		return fmt.Errorf("disk size cannot exceed 10 TB")
	}

	// Validate PXE config if provided
	if cfg.ImageType == "pxe" {
		if cfg.PXEConfig == nil {
			return fmt.Errorf("PXE configuration is required when imageType is 'pxe'")
		}
		if cfg.PXEConfig.KernelURL == "" {
			return fmt.Errorf("kernel URL is required for PXE boot")
		}
		if cfg.PXEConfig.InitrdURL == "" {
			return fmt.Errorf("initrd URL is required for PXE boot")
		}
		// Validate URLs are well-formed
		if !strings.HasPrefix(cfg.PXEConfig.KernelURL, "http://") && !strings.HasPrefix(cfg.PXEConfig.KernelURL, "https://") {
			return fmt.Errorf("kernel URL must be a valid HTTP/HTTPS URL")
		}
		if !strings.HasPrefix(cfg.PXEConfig.InitrdURL, "http://") && !strings.HasPrefix(cfg.PXEConfig.InitrdURL, "https://") {
			return fmt.Errorf("initrd URL must be a valid HTTP/HTTPS URL")
		}
	}

	// Validate cloud-init config if provided
	if cfg.CloudInit != nil {
		if err := validateCloudInitConfig(cfg.CloudInit); err != nil {
			return fmt.Errorf("invalid cloud-init config: %w", err)
		}
	}

	return nil
}

// validateCloudInitConfig validates cloud-init configuration
func validateCloudInitConfig(cfg *core.CloudInitConfig) error {
	if cfg.CommonFields.Hostname != "" {
		if len(cfg.CommonFields.Hostname) > 64 {
			return fmt.Errorf("hostname must be 64 characters or less")
		}
		hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
		if !hostnameRegex.MatchString(cfg.CommonFields.Hostname) {
			return fmt.Errorf("hostname can only contain letters, numbers, dots, and hyphens")
		}
	}

	if cfg.CommonFields.Username != "" {
		if len(cfg.CommonFields.Username) > 32 {
			return fmt.Errorf("username must be 32 characters or less")
		}
		usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
		if !usernameRegex.MatchString(cfg.CommonFields.Username) {
			return fmt.Errorf("username can only contain letters, numbers, hyphens, and underscores")
		}
	}

	if len(cfg.CommonFields.Password) > 256 {
		return fmt.Errorf("password must be 256 characters or less")
	}
	if strings.ContainsAny(cfg.CommonFields.Password, "\r\n\x00") {
		return fmt.Errorf("password cannot contain control characters")
	}

	if len(cfg.CommonFields.Packages) > 50 {
		return fmt.Errorf("cannot specify more than 50 packages")
	}

	for _, pkg := range cfg.CommonFields.Packages {
		if len(pkg) > 100 {
			return fmt.Errorf("package name must be 100 characters or less")
		}
		packageRegex := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
		if !packageRegex.MatchString(pkg) {
			return fmt.Errorf("package name can only contain letters, numbers, dots, hyphens, and underscores")
		}
	}

	return nil
}

// validateUUID validates UUID format
func validateUUID(uuid string) error {
	if uuid == "" {
		return fmt.Errorf("UUID is required")
	}
	uuidRegex := regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("invalid UUID format")
	}
	return nil
}

func validInterfaceName(name string) bool {
	if len(name) == 0 || len(name) > 15 || strings.HasPrefix(name, "-") {
		return false
	}
	return linuxInterfaceNamePattern.MatchString(name)
}

// validateFilePath validates file path for security
func validateFilePath(path string) error {
	return validateFilePathWithin(path, []string{
		"/var/lib/flint/images",
		"/var/lib/flint/image-repository",
		"/var/lib/libvirt/images",
	})
}

func validateFilePathWithin(path string, allowedRoots []string) error {
	if path == "" {
		return fmt.Errorf("file path is required")
	}
	if len(path) > 4096 {
		return fmt.Errorf("file path is too long")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("file path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("file path does not exist")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("file path must reference a regular file")
	}
	allowed := false
	for _, root := range allowedRoots {
		rel, relErr := filepath.Rel(root, resolved)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("file must be inside an approved image directory")
	}
	return nil
}

// sendError sends a consistent error response
func sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// sendInternalError sends a generic internal error without exposing details
func sendInternalError(w http.ResponseWriter, err error) {
	// Log the actual error for debugging (you might want to use a proper logger)
	fmt.Printf("Internal error: %v\n", err)
	sendError(w, "Internal server error", http.StatusInternalServerError)
}

func (s *Server) handleCreateVM() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg core.VMCreationConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		// Validate input
		if err := validateVMCreationConfig(&cfg); err != nil {
			sendError(w, err.Error(), http.StatusBadRequest)
			return
		}

		vm, err := s.client.CreateVM(cfg)
		if err != nil {
			// Don't expose internal error details that could be sensitive
			http.Error(w, `{"error": "Failed to create VM"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vm)
	}
}

func (s *Server) handleGetVolumes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolName := chi.URLParam(r, "poolName")
		volumes, err := s.client.GetVolumes(poolName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(volumes)
	}
}

func (s *Server) handleCreateVolume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolName := chi.URLParam(r, "poolName")

		var req core.VolumeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		err := s.client.CreateVolume(poolName, req)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func (s *Server) handleUpdateVolume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolName := chi.URLParam(r, "poolName")
		volumeName := chi.URLParam(r, "volumeName")

		if poolName == "" || volumeName == "" {
			http.Error(w, `{"error": "Pool name and volume name are required"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			SizeGB int `json:"size_gb"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}
		if req.SizeGB <= 0 || req.SizeGB > 16384 {
			http.Error(w, `{"error": "size_gb must be between 1 and 16384"}`, http.StatusBadRequest)
			return
		}

		config := core.VolumeConfig{
			Name:   volumeName,
			SizeGB: uint64(req.SizeGB),
		}
		err := s.client.UpdateVolume(poolName, volumeName, config)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to update volume: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}
}

func (s *Server) handleDeleteVolume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolName := chi.URLParam(r, "poolName")
		volumeName := chi.URLParam(r, "volumeName")

		if poolName == "" || volumeName == "" {
			http.Error(w, `{"error": "Pool name and volume name are required"}`, http.StatusBadRequest)
			return
		}

		err := s.client.DeleteVolume(poolName, volumeName)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to delete volume: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}
}

func (s *Server) handleCreateNetwork() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string `json:"name"`
			BridgeName string `json:"bridgeName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		err := s.client.CreateNetwork(req.Name, req.BridgeName)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func (s *Server) handleCreateBridge() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name  string   `json:"name"`
			Ports []string `json:"ports"`
			STP   bool     `json:"stp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		if !validInterfaceName(req.Name) {
			http.Error(w, `{"error": "Invalid bridge name"}`, http.StatusBadRequest)
			return
		}
		if len(req.Ports) > 64 {
			http.Error(w, `{"error": "Too many bridge ports"}`, http.StatusBadRequest)
			return
		}
		for _, port := range req.Ports {
			if !validInterfaceName(port) {
				http.Error(w, `{"error": "Invalid bridge port name"}`, http.StatusBadRequest)
				return
			}
		}

		// Create the bridge
		if err := exec.Command("ip", "link", "add", "name", req.Name, "type", "bridge").Run(); err != nil {
			http.Error(w, `{"error": "Failed to create bridge: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		// Add ports to the bridge
		for _, port := range req.Ports {
			if err := exec.Command("ip", "link", "set", port, "master", req.Name).Run(); err != nil {
				// Log error but continue with other ports
				fmt.Printf("Warning: Failed to add port %s to bridge %s: %v\n", port, req.Name, err)
			}
		}

		// Enable STP if requested
		if req.STP {
			exec.Command("ip", "link", "set", req.Name, "type", "bridge", "stp_state", "1").Run()
		}

		// Bring the bridge up
		if err := exec.Command("ip", "link", "set", req.Name, "up").Run(); err != nil {
			fmt.Printf("Warning: Failed to bring bridge %s up: %v\n", req.Name, err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("Bridge %s created successfully", req.Name),
		})
	}
}

func (s *Server) handleAttachDiskToVM() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		var req struct {
			VolumePath string `json:"volumePath"`
			TargetDev  string `json:"targetDev"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		err := s.client.AttachDiskToVM(uuid, req.VolumePath, req.TargetDev)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Disk attached successfully"})
	}
}

func (s *Server) handleAttachNetworkInterfaceToVM() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")

		// Support both old and new request formats
		var req struct {
			// Old format
			NetworkName string `json:"networkName"`
			Model       string `json:"model"`
			// New format
			InterfaceType string `json:"interfaceType"`
			Source        string `json:"source"`
			MacAddress    string `json:"macAddress"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON in request body"}`, http.StatusBadRequest)
			return
		}

		// Determine which format is being used
		var networkName, model string
		if req.InterfaceType != "" && req.Source != "" {
			// New format
			networkName = req.Source
			model = req.Model
		} else {
			// Old format
			networkName = req.NetworkName
			model = req.Model
		}

		if networkName == "" || model == "" {
			http.Error(w, `{"error": "Missing required fields: source/networkName and model"}`, http.StatusBadRequest)
			return
		}

		// For now, just try to attach the interface directly
		// TODO: Add proper VM state management when hot-plug is needed
		err := s.client.AttachNetworkInterfaceToVM(uuid, networkName, model)
		if err != nil {
			http.Error(w, `{"error": "Failed to attach network interface: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Network interface attached successfully"})
	}
}
func (s *Server) handleDetectSSHKey() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			sendInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"keys": detectSSHKeys(homeDir)})
	}
}

type sshKeyInfo struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Secure bool   `json:"secure"`
}

// detectSSHKeys returns metadata for conventional private key files. It never
// reads or returns key contents.
func detectSSHKeys(homeDir string) []sshKeyInfo {
	keys := make([]sshKeyInfo, 0)
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		path := filepath.Join(homeDir, ".ssh", name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		keys = append(keys, sshKeyInfo{
			Path:   path,
			Name:   name,
			Secure: info.Mode().Perm()&0077 == 0,
		})
	}
	return keys
}

// handleGetVMConsoleStream returns WebSocket connection info for console streaming
func (s *Server) handleGetVMConsoleStream() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		if uuid == "" {
			http.Error(w, "UUID is required", http.StatusBadRequest)
			return
		}

		// Return WebSocket path for frontend to connect to
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"websocket_path": fmt.Sprintf("/api/vms/%s/serial-console/ws", uuid),
		})
	})
}

// handleCreateVMFromTemplate creates a VM from a template
func (s *Server) handleCreateVMFromTemplate() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TemplateID string `json:"templateId"`
			Name       string `json:"name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Get snapshots for the template VM
		snapshots, err := s.client.GetVMSnapshots(req.TemplateID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get template snapshots: %v", err), http.StatusInternalServerError)
			return
		}

		if len(snapshots) == 0 {
			http.Error(w, "No snapshots found for template", http.StatusBadRequest)
			return
		}

		// Use the latest snapshot
		latestSnapshot := snapshots[0]

		// Create VM from snapshot (simplified - in practice you'd clone the VM)
		err = s.client.RevertToVMSnapshot(req.TemplateID, latestSnapshot.Name)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create VM from template: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":    req.TemplateID, // In practice, this would be a new UUID
			"name":    req.Name,
			"message": "VM created from template successfully",
		})
	})
}

// handleGetVMTemplates returns available VM templates
func (s *Server) handleGetVMTemplates() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get all VMs and check which ones have snapshots (templates)
		vms, err := s.client.GetVMSummaries()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get VMs: %v", err), http.StatusInternalServerError)
			return
		}

		var templates []map[string]interface{}

		for _, vm := range vms {
			snapshots, err := s.client.GetVMSnapshots(vm.UUID)
			if err != nil {
				continue // Skip VMs we can't get snapshots for
			}

			if len(snapshots) > 0 {
				// This VM has snapshots, so it can be used as a template
				template := map[string]interface{}{
					"id":          vm.UUID,
					"name":        vm.Name + "-template",
					"description": fmt.Sprintf("Template based on %s with %d snapshots", vm.Name, len(snapshots)),
					"sourceVM":    vm.Name,
					"vcpus":       vm.VCPUs,
					"memory":      vm.MemoryKB / 1024, // Convert to MB
					"diskSize":    20,                 // Default disk size
					"createdAt":   time.Now().Format(time.RFC3339),
				}
				templates = append(templates, template)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(templates)
	})
}

// handleCreateVMTemplate creates a new template from a VM
func (s *Server) handleCreateVMTemplate() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			VMID        string `json:"vmId"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Create a snapshot of the VM to use as template
		snapshotReq := core.CreateSnapshotRequest{
			Name:        req.Name,
			Description: req.Description,
		}

		snapshot, err := s.client.CreateVMSnapshot(req.VMID, snapshotReq)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create template snapshot: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          req.VMID,
			"name":        req.Name,
			"description": req.Description,
			"snapshot":    snapshot.Name,
			"createdAt":   time.Now().Format(time.RFC3339),
			"message":     "Template created successfully",
		})
	})
}

// handleUpdateNetwork updates a virtual network (start/stop/restart)
func (s *Server) handleUpdateNetwork() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkName := chi.URLParam(r, "networkName")
		if networkName == "" {
			http.Error(w, "Network name is required", http.StatusBadRequest)
			return
		}

		var req struct {
			Action string `json:"action"` // "start", "stop", "restart"
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		err := s.client.UpdateNetwork(networkName, req.Action)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to update network: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	})
}

// handleDeleteNetwork deletes a virtual network
func (s *Server) handleDeleteNetwork() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		networkName := chi.URLParam(r, "networkName")
		if networkName == "" {
			http.Error(w, "Network name is required", http.StatusBadRequest)
			return
		}

		// Prevent deletion of default network
		if networkName == "default" {
			http.Error(w, "Cannot delete default network", http.StatusBadRequest)
			return
		}

		// Call the actual delete function
		err := s.client.DeleteNetwork(networkName)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to delete network: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("Network '%s' deleted successfully", networkName),
		})
	})
}

// handleDeleteImage deletes an image
func (s *Server) handleDeleteImage() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imageId := chi.URLParam(r, "imageId")
		if imageId == "" {
			http.Error(w, `{"error": "Image ID is required"}`, http.StatusBadRequest)
			return
		}

		// Call the actual delete function
		err := s.client.DeleteImage(imageId)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to delete image: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// handleGetVMVNCInfo returns VNC connection details for a VM
func (s *Server) handleGetVMVNCInfo() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")
		if err := validateUUID(uuid); err != nil {
			sendError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := s.client.GetVMVNCInfo(uuid); err != nil {
			http.Error(w, `{"error": "Failed to get VNC info"}`, http.StatusInternalServerError)
			return
		}
		token, err := s.issueConsoleToken(uuid, "vnc")
		if err != nil {
			sendInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"websocket_path": fmt.Sprintf("/api/vms/%s/vnc/ws", uuid),
			"token":          token,
		})
	})
}

// handleVMVNCWebSocket proxies VNC protocol over WebSocket
func (s *Server) handleVMVNCWebSocket() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := chi.URLParam(r, "uuid")

		// Authenticate using token from query parameters
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Authentication token required", http.StatusUnauthorized)
			return
		}

		if !s.consumeConsoleToken(token, uuid, "vnc") {
			http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
			return
		}

		// Get VNC connection details
		vncInfo, err := s.client.GetVMVNCInfo(uuid)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get VNC info: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		// Upgrade HTTP connection to WebSocket
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return sameWebSocketOrigin(r, s.trustedOrigins)
			},
		}

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade to WebSocket: %v", err)
			return
		}
		defer wsConn.Close()

		// Connect to VNC server
		vncAddr := net.JoinHostPort(vncInfo.Host, vncInfo.Port)
		vncConn, err := net.Dial("tcp", vncAddr)
		if err != nil {
			log.Printf("Failed to connect to VNC server at %s: %v", vncAddr, err)
			wsConn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to VNC server"))
			return
		}
		defer vncConn.Close()

		log.Printf("VNC WebSocket proxy established for VM %s (VNC at %s)", uuid, vncAddr)

		// Bidirectional proxy between WebSocket and VNC
		errChan := make(chan error, 2)

		// VNC -> WebSocket (read from VNC, write to WebSocket)
		go func() {
			buffer := make([]byte, 8192)
			for {
				n, err := vncConn.Read(buffer)
				if err != nil {
					if err != io.EOF {
						log.Printf("VNC read error: %v", err)
					}
					errChan <- err
					return
				}

				if err := wsConn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
					log.Printf("WebSocket write error: %v", err)
					errChan <- err
					return
				}
			}
		}()

		// WebSocket -> VNC (read from WebSocket, write to VNC)
		go func() {
			for {
				msgType, data, err := wsConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						log.Printf("WebSocket read error: %v", err)
					}
					errChan <- err
					return
				}

				// Only forward binary messages (VNC protocol is binary)
				if msgType == websocket.BinaryMessage {
					if _, err := vncConn.Write(data); err != nil {
						log.Printf("VNC write error: %v", err)
						errChan <- err
						return
					}
				}
			}
		}()

		// Wait for either direction to error/close
		<-errChan
		log.Printf("VNC WebSocket proxy closed for VM %s", uuid)
	}
}

// ========== NWFilter / Firewall Handlers ==========

// handleListNWFilters lists all network filters
func (s *Server) handleListNWFilters() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters, err := s.client.ListNWFilters()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(filters)
	}
}

// handleGetNWFilter gets a specific network filter
func (s *Server) handleGetNWFilter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "filter name required", http.StatusBadRequest)
			return
		}

		filter, err := s.client.GetNWFilter(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(filter)
	}
}

// handleCreateNWFilter creates a new network filter
func (s *Server) handleCreateNWFilter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req core.CreateNWFilterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Name == "" {
			http.Error(w, "filter name required", http.StatusBadRequest)
			return
		}

		if err := s.client.CreateNWFilter(req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "filter created"})
	}
}

// handleUpdateNWFilter updates an existing network filter
func (s *Server) handleUpdateNWFilter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "filter name required", http.StatusBadRequest)
			return
		}

		var req core.CreateNWFilterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := s.client.UpdateNWFilter(name, req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "filter updated"})
	}
}

// handleDeleteNWFilter deletes a network filter
func (s *Server) handleDeleteNWFilter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "filter name required", http.StatusBadRequest)
			return
		}

		if err := s.client.DeleteNWFilter(name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "filter deleted"})
	}
}
