package serverregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/volantvm/flint/pkg/core"
)

// Registry manages multiple server configurations
type Registry struct {
	servers       map[string]*core.ServerConfig
	mu            sync.RWMutex
	storagePath   string
	defaultServer string
}

// NewRegistry creates a new server registry
func NewRegistry(storagePath string) (*Registry, error) {
	if storagePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		storagePath = filepath.Join(homeDir, ".flint", "servers.json")
	}

	registry := &Registry{
		servers:     make(map[string]*core.ServerConfig),
		storagePath: storagePath,
	}

	// Load existing servers from storage
	if err := registry.load(); err != nil {
		// If file doesn't exist, start with empty registry
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load server registry: %w", err)
		}
	}

	return registry, nil
}

// AddServer adds a new server to the registry
func (r *Registry) AddServer(req core.CreateServerRequest) (*core.ServerConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate unique ID
	id := uuid.New().String()

	// Set defaults
	if req.ImagePoolPath == "" {
		req.ImagePoolPath = "/var/lib/flint/images"
	}
	if req.ConnectionRetry == 0 {
		req.ConnectionRetry = 3
	}
	if req.Timeout == 0 {
		req.Timeout = 30
	}

	now := time.Now()
	server := &core.ServerConfig{
		ID:              id,
		Name:            req.Name,
		Description:     req.Description,
		URI:             req.URI,
		ISOPool:         req.ISOPool,
		TemplatePool:    req.TemplatePool,
		ImagePoolPath:   req.ImagePoolPath,
		IsDefault:       req.IsDefault,
		IsActive:        false, // Will be determined by connection test
		Tags:            req.Tags,
		SSHKeyPath:      req.SSHKeyPath,
		SSHUser:         req.SSHUser,
		ConnectionRetry: req.ConnectionRetry,
		Timeout:         req.Timeout,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// If this is set as default, unset other defaults
	if server.IsDefault {
		for _, s := range r.servers {
			s.IsDefault = false
		}
		r.defaultServer = id
	}

	// If this is the first server, make it default
	if len(r.servers) == 0 {
		server.IsDefault = true
		r.defaultServer = id
	}

	r.servers[id] = server

	// Save to storage
	if err := r.save(); err != nil {
		return nil, fmt.Errorf("failed to save server registry: %w", err)
	}

	return server, nil
}

// UpdateServer updates an existing server configuration
func (r *Registry) UpdateServer(id string, req core.UpdateServerRequest) (*core.ServerConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return nil, fmt.Errorf("server not found: %s", id)
	}

	// Update fields if provided
	if req.Name != nil {
		server.Name = *req.Name
	}
	if req.Description != nil {
		server.Description = *req.Description
	}
	if req.URI != nil {
		server.URI = *req.URI
	}
	if req.ISOPool != nil {
		server.ISOPool = *req.ISOPool
	}
	if req.TemplatePool != nil {
		server.TemplatePool = *req.TemplatePool
	}
	if req.ImagePoolPath != nil {
		server.ImagePoolPath = *req.ImagePoolPath
	}
	if req.Tags != nil {
		server.Tags = *req.Tags
	}
	if req.SSHKeyPath != nil {
		server.SSHKeyPath = *req.SSHKeyPath
	}
	if req.SSHUser != nil {
		server.SSHUser = *req.SSHUser
	}
	if req.ConnectionRetry != nil {
		server.ConnectionRetry = *req.ConnectionRetry
	}
	if req.Timeout != nil {
		server.Timeout = *req.Timeout
	}
	if req.IsDefault != nil && *req.IsDefault {
		// Unset other defaults
		for _, s := range r.servers {
			s.IsDefault = false
		}
		server.IsDefault = true
		r.defaultServer = id
	}

	server.UpdatedAt = time.Now()

	// Save to storage
	if err := r.save(); err != nil {
		return nil, fmt.Errorf("failed to save server registry: %w", err)
	}

	return server, nil
}

// DeleteServer removes a server from the registry
func (r *Registry) DeleteServer(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return fmt.Errorf("server not found: %s", id)
	}

	// Don't allow deletion of the only server
	if len(r.servers) == 1 {
		return fmt.Errorf("cannot delete the only server")
	}

	// If deleting default server, assign a new default
	if server.IsDefault {
		delete(r.servers, id)
		// Set first remaining server as default
		for newDefaultID := range r.servers {
			r.servers[newDefaultID].IsDefault = true
			r.defaultServer = newDefaultID
			break
		}
	} else {
		delete(r.servers, id)
	}

	// Save to storage
	if err := r.save(); err != nil {
		return fmt.Errorf("failed to save server registry: %w", err)
	}

	return nil
}

// GetServer retrieves a server by ID
func (r *Registry) GetServer(id string) (*core.ServerConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	server, exists := r.servers[id]
	if !exists {
		return nil, fmt.Errorf("server not found: %s", id)
	}

	return server, nil
}

// GetDefaultServer returns the default server
func (r *Registry) GetDefaultServer() (*core.ServerConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultServer == "" {
		return nil, fmt.Errorf("no default server configured")
	}

	server, exists := r.servers[r.defaultServer]
	if !exists {
		return nil, fmt.Errorf("default server not found")
	}

	return server, nil
}

// ListServers returns all registered servers
func (r *Registry) ListServers() []*core.ServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]*core.ServerConfig, 0, len(r.servers))
	for _, server := range r.servers {
		servers = append(servers, server)
	}

	return servers
}

// SetDefault sets a server as the default
func (r *Registry) SetDefault(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return fmt.Errorf("server not found: %s", id)
	}

	// Unset all other defaults
	for _, s := range r.servers {
		s.IsDefault = false
	}

	server.IsDefault = true
	r.defaultServer = id

	// Save to storage
	if err := r.save(); err != nil {
		return fmt.Errorf("failed to save server registry: %w", err)
	}

	return nil
}

// UpdateServerStatus updates the active status and last checked time
func (r *Registry) UpdateServerStatus(id string, isActive bool, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return fmt.Errorf("server not found: %s", id)
	}

	server.IsActive = isActive
	server.LastChecked = time.Now()

	// Save to storage
	if err := r.save(); err != nil {
		return fmt.Errorf("failed to save server registry: %w", err)
	}

	return nil
}

// load reads server configurations from storage
func (r *Registry) load() error {
	data, err := os.ReadFile(r.storagePath)
	if err != nil {
		return err
	}

	var stored struct {
		Servers       map[string]*core.ServerConfig `json:"servers"`
		DefaultServer string                        `json:"default_server"`
	}

	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to unmarshal server registry: %w", err)
	}

	r.servers = stored.Servers
	r.defaultServer = stored.DefaultServer

	return nil
}

// save writes server configurations to storage
func (r *Registry) save() error {
	// Ensure directory exists
	dir := filepath.Dir(r.storagePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	stored := struct {
		Servers       map[string]*core.ServerConfig `json:"servers"`
		DefaultServer string                        `json:"default_server"`
	}{
		Servers:       r.servers,
		DefaultServer: r.defaultServer,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal server registry: %w", err)
	}

	if err := os.WriteFile(r.storagePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write server registry: %w", err)
	}

	return nil
}

// GetServerCount returns the number of registered servers
func (r *Registry) GetServerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.servers)
}

// MigrateFromConfig creates a default server entry from existing config
func (r *Registry) MigrateFromConfig(uri, isoPool, templatePool, imagePoolPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if we already have servers
	if len(r.servers) > 0 {
		return nil // Already migrated
	}

	// Create default server from config
	id := uuid.New().String()
	now := time.Now()

	server := &core.ServerConfig{
		ID:              id,
		Name:            "Local Server",
		Description:     "Default local libvirt server (migrated from config)",
		URI:             uri,
		ISOPool:         isoPool,
		TemplatePool:    templatePool,
		ImagePoolPath:   imagePoolPath,
		IsDefault:       true,
		IsActive:        false,
		Tags:            []string{"local", "default"},
		ConnectionRetry: 3,
		Timeout:         30,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	r.servers[id] = server
	r.defaultServer = id

	// Save to storage
	if err := r.save(); err != nil {
		return fmt.Errorf("failed to save migrated server: %w", err)
	}

	return nil
}
