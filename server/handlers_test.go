package server

import (
	"github.com/volantvm/flint/pkg/core"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateVMCreationConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  core.VMCreationConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: core.VMCreationConfig{
				Name:       "test-vm",
				MemoryMB:   2048,
				VCPUs:      2,
				ImageName:  "ubuntu-24.04",
				ImageType:  "template",
				DiskSizeGB: 20,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			config: core.VMCreationConfig{
				Name:       "",
				MemoryMB:   2048,
				VCPUs:      2,
				ImageName:  "ubuntu-24.04",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
		{
			name: "invalid name characters",
			config: core.VMCreationConfig{
				Name:       "test vm@",
				MemoryMB:   2048,
				VCPUs:      2,
				ImageName:  "ubuntu-24.04",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
		{
			name: "zero memory",
			config: core.VMCreationConfig{
				Name:       "test-vm",
				MemoryMB:   0,
				VCPUs:      2,
				ImageName:  "ubuntu-24.04",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
		{
			name: "excessive memory",
			config: core.VMCreationConfig{
				Name:       "test-vm",
				MemoryMB:   600000, // 600 GB
				VCPUs:      2,
				ImageName:  "ubuntu-24.04",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
		{
			name: "zero vcpus",
			config: core.VMCreationConfig{
				Name:       "test-vm",
				MemoryMB:   2048,
				VCPUs:      0,
				ImageName:  "ubuntu-24.04",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
		{
			name: "excessive vcpus",
			config: core.VMCreationConfig{
				Name:       "test-vm",
				MemoryMB:   2048,
				VCPUs:      200,
				ImageName:  "ubuntu-24.04",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
		{
			name: "empty image name",
			config: core.VMCreationConfig{
				Name:       "test-vm",
				MemoryMB:   2048,
				VCPUs:      2,
				ImageName:  "",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
		{
			name: "invalid image type",
			config: core.VMCreationConfig{
				Name:       "test-vm",
				MemoryMB:   2048,
				VCPUs:      2,
				ImageName:  "ubuntu-24.04",
				ImageType:  "invalid",
				DiskSizeGB: 20,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVMCreationConfig(&tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVMCreationConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "valid UUID",
			uuid:    "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "empty UUID",
			uuid:    "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			uuid:    "invalid-uuid",
			wantErr: true,
		},
		{
			name:    "too short",
			uuid:    "550e8400-e29b-41d4-a716",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUUID(tt.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUUID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	allowedRoot := t.TempDir()
	validPath := filepath.Join(allowedRoot, "ubuntu.iso")
	if err := os.WriteFile(validPath, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid absolute path",
			path:    validPath,
			wantErr: false,
		},
		{
			name:    "relative path rejected",
			path:    "./images/ubuntu.iso",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "directory traversal",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "too long path",
			path:    string(make([]byte, 5000)), // Very long path
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePathWithin(tt.path, []string{allowedRoot})
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
