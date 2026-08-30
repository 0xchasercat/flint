package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/volantvm/flint/pkg/config"
	"github.com/volantvm/flint/pkg/libvirtclient"
	"github.com/volantvm/flint/pkg/logger"
)

// ConnectionConfigRequest represents a request to update connection configuration
type ConnectionConfigRequest struct {
	URI         string `json:"uri"`
	SSHEnabled  bool   `json:"ssh_enabled"`
	SSHUsername string `json:"ssh_username"`
	SSHHost     string `json:"ssh_host"`
	SSHPort     int    `json:"ssh_port"`
	SSHKeyPath  string `json:"ssh_key_path"`
}

// ConnectionStatusResponse represents the current connection status
type ConnectionStatusResponse struct {
	Connected    bool   `json:"connected"`
	URI          string `json:"uri"`
	EffectiveURI string `json:"effective_uri"`
	SSHEnabled   bool   `json:"ssh_enabled"`
	SSHHost      string `json:"ssh_host,omitempty"`
	SSHUsername  string `json:"ssh_username,omitempty"`
	SSHPort      int    `json:"ssh_port,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ConnectionTestRequest represents a request to test connection parameters
type ConnectionTestRequest struct {
	URI         string `json:"uri"`
	SSHEnabled  bool   `json:"ssh_enabled"`
	SSHUsername string `json:"ssh_username"`
	SSHHost     string `json:"ssh_host"`
	SSHPort     int    `json:"ssh_port"`
	SSHKeyPath  string `json:"ssh_key_path"`
}

// ConnectionTestResponse represents the result of testing a connection
type ConnectionTestResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	EffectiveURI string `json:"effective_uri,omitempty"`
}

// handleGetConnectionStatus returns the current connection status
func (s *Server) handleGetConnectionStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := config.LoadConfig("")
		if err != nil {
			logger.Error("Failed to load config", map[string]interface{}{
				"error": err.Error(),
			})
			http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
			return
		}

		effectiveURI := cfg.GetEffectiveLibvirtURI()

		// Check if connection is working by trying to get host status
		connected := true
		errorMessage := ""
		_, err = s.client.GetHostStatus()
		if err != nil {
			connected = false
			errorMessage = err.Error()
		}

		response := ConnectionStatusResponse{
			Connected:    connected,
			URI:          cfg.Libvirt.URI,
			EffectiveURI: effectiveURI,
			SSHEnabled:   cfg.Libvirt.SSH.Enabled,
			ErrorMessage: errorMessage,
		}

		if cfg.Libvirt.SSH.Enabled {
			response.SSHHost = cfg.Libvirt.SSH.Host
			response.SSHUsername = cfg.Libvirt.SSH.Username
			response.SSHPort = cfg.Libvirt.SSH.Port
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// handleTestConnection tests connection parameters without saving them
func (s *Server) handleTestConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ConnectionTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Build test URI
		testURI := req.URI
		if req.SSHEnabled {
			if req.SSHUsername == "" || req.SSHHost == "" {
				response := ConnectionTestResponse{
					Success: false,
					Message: "SSH username and host are required when SSH is enabled",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
				return
			}

			// Validate SSH connection parameters
			port := req.SSHPort
			if port == 0 {
				port = 22
			}

			if err := libvirtclient.ValidateSSHConnection(req.SSHUsername, req.SSHHost, port, req.SSHKeyPath); err != nil {
				response := ConnectionTestResponse{
					Success: false,
					Message: fmt.Sprintf("SSH validation failed: %v", err),
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
				return
			}

			// Build SSH URI
			testURI = fmt.Sprintf("qemu+ssh://%s@%s", req.SSHUsername, req.SSHHost)
			if port != 22 {
				testURI = fmt.Sprintf("%s:%d", testURI, port)
			}
			testURI = testURI + "/system"
		}

		// Try to connect with test URI
		testClient, err := libvirtclient.NewClient(testURI, "", "")
		if err != nil {
			response := ConnectionTestResponse{
				Success:      false,
				Message:      fmt.Sprintf("Connection failed: %v", err),
				EffectiveURI: testURI,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
		defer testClient.Close()

		// Try to get host status to verify connection works
		_, err = testClient.GetHostStatus()
		if err != nil {
			response := ConnectionTestResponse{
				Success:      false,
				Message:      fmt.Sprintf("Connection established but failed to query host: %v", err),
				EffectiveURI: testURI,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		response := ConnectionTestResponse{
			Success:      true,
			Message:      "Connection successful",
			EffectiveURI: testURI,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// handleUpdateConnectionConfig updates the connection configuration
func (s *Server) handleUpdateConnectionConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ConnectionConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Load current config
		cfg, err := config.LoadConfig("")
		if err != nil {
			http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
			return
		}

		// Update libvirt config
		if req.URI != "" {
			cfg.Libvirt.URI = req.URI
		}

		cfg.Libvirt.SSH.Enabled = req.SSHEnabled
		if req.SSHEnabled {
			if req.SSHUsername == "" || req.SSHHost == "" {
				http.Error(w, "SSH username and host are required when SSH is enabled", http.StatusBadRequest)
				return
			}

			cfg.Libvirt.SSH.Username = req.SSHUsername
			cfg.Libvirt.SSH.Host = req.SSHHost

			if req.SSHPort > 0 {
				cfg.Libvirt.SSH.Port = req.SSHPort
			} else {
				cfg.Libvirt.SSH.Port = 22
			}

			if req.SSHKeyPath != "" {
				cfg.Libvirt.SSH.KeyPath = req.SSHKeyPath
			}
		}

		// Validate configuration
		if err := cfg.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("Invalid configuration: %v", err), http.StatusBadRequest)
			return
		}

		// Save configuration
		configDir := filepath.Join(os.Getenv("HOME"), ".flint")
		configPath := filepath.Join(configDir, "config.json")

		if err := os.MkdirAll(configDir, 0700); err != nil {
			http.Error(w, "Failed to create config directory", http.StatusInternalServerError)
			return
		}

		if err := cfg.SaveConfig(configPath); err != nil {
			http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
			return
		}

		logger.Info("Connection configuration updated", map[string]interface{}{
			"ssh_enabled": cfg.Libvirt.SSH.Enabled,
			"uri":         cfg.GetEffectiveLibvirtURI(),
		})

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":       true,
			"message":       "Configuration updated. Please restart the server for changes to take effect.",
			"effective_uri": cfg.GetEffectiveLibvirtURI(),
		})
	}
}
