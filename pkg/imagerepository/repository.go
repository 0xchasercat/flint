package imagerepository

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// CloudImage represents a downloadable cloud image
type CloudImage struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	URL          string  `json:"url"`
	ChecksumURL  string  `json:"checksum_url,omitempty"`
	SizeGB       float64 `json:"size_gb"`
	Type         string  `json:"type"`
	OS           string  `json:"os"`
	Version      string  `json:"version"`
	Description  string  `json:"description"`
	Architecture string  `json:"architecture"`
}

// ImageRepository manages cloud image downloads
type ImageRepository struct {
	StoragePath string
	Images      []CloudImage
}

// NewImageRepository creates a new image repository
func NewImageRepository(storagePath string) *ImageRepository {
	return &ImageRepository{
		StoragePath: storagePath,
		Images:      getDefaultImages(),
	}
}

// getDefaultImages returns curated list of popular cloud images
func getDefaultImages() []CloudImage {
	return []CloudImage{
		{
			ID:           "ubuntu-26.04-lts",
			Name:         "Ubuntu 26.04 LTS (Resolute Raccoon)",
			URL:          "https://cloud-images.ubuntu.com/releases/resolute/release/ubuntu-26.04-server-cloudimg-amd64.img",
			ChecksumURL:  "https://cloud-images.ubuntu.com/releases/resolute/release/SHA256SUMS",
			SizeGB:       0.9,
			Type:         "template",
			OS:           "Ubuntu",
			Version:      "26.04 LTS",
			Description:  "Ubuntu 26.04 LTS (Resolute Raccoon) cloud image with cloud-init support",
			Architecture: "amd64",
		},
		{
			ID:           "ubuntu-24.04-lts",
			Name:         "Ubuntu 24.04 LTS (Noble)",
			URL:          "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
			ChecksumURL:  "https://cloud-images.ubuntu.com/noble/current/SHA256SUMS",
			SizeGB:       2.2,
			Type:         "template",
			OS:           "Ubuntu",
			Version:      "24.04 LTS",
			Description:  "Latest Ubuntu 24.04 LTS cloud image with cloud-init support",
			Architecture: "amd64",
		},
		{
			ID:           "ubuntu-22.04-lts",
			Name:         "Ubuntu 22.04 LTS (Jammy)",
			URL:          "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
			ChecksumURL:  "https://cloud-images.ubuntu.com/jammy/current/SHA256SUMS",
			SizeGB:       2.1,
			Type:         "template",
			OS:           "Ubuntu",
			Version:      "22.04 LTS",
			Description:  "Ubuntu 22.04 LTS cloud image with cloud-init support",
			Architecture: "amd64",
		},
		{
			ID:           "debian-12",
			Name:         "Debian 12 (Bookworm)",
			URL:          "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2",
			SizeGB:       2.0,
			Type:         "template",
			OS:           "Debian",
			Version:      "12",
			Description:  "Debian 12 (Bookworm) cloud image",
			Architecture: "amd64",
		},
		{
			ID:           "centos-stream-9",
			Name:         "CentOS Stream 9",
			URL:          "https://cloud.centos.org/centos/9-stream/x86_64/images/CentOS-Stream-GenericCloud-9-latest.x86_64.qcow2",
			SizeGB:       1.5,
			Type:         "template",
			OS:           "CentOS",
			Version:      "Stream 9",
			Description:  "CentOS Stream 9 cloud image",
			Architecture: "x86_64",
		},
		{
			ID:           "fedora-39",
			Name:         "Fedora 39",
			URL:          "https://download.fedoraproject.org/pub/fedora/linux/releases/39/Cloud/x86_64/images/Fedora-Cloud-Base-39-1.5.x86_64.qcow2",
			SizeGB:       1.8,
			Type:         "template",
			OS:           "Fedora",
			Version:      "39",
			Description:  "Fedora 39 cloud base image",
			Architecture: "x86_64",
		},
		{
			ID:           "ubuntu-25.04",
			Name:         "Ubuntu 25.04 (Plucky)",
			URL:          "https://cloud-images.ubuntu.com/releases/plucky/release/ubuntu-25.04-server-cloudimg-amd64.img",
			ChecksumURL:  "https://cloud-images.ubuntu.com/releases/plucky/release/SHA256SUMS",
			SizeGB:       2.3,
			Type:         "template",
			OS:           "Ubuntu",
			Version:      "25.04",
			Description:  "Ubuntu 25.04 (Plucky) cloud image with cloud-init support",
			Architecture: "amd64",
		},
		{
			ID:           "debian-13",
			Name:         "Debian 13 (Trixie)",
			URL:          "https://cloud.debian.org/images/cloud/trixie/20250814-2204/debian-13-generic-amd64-20250814-2204.qcow2",
			SizeGB:       2.1,
			Type:         "template",
			OS:           "Debian",
			Version:      "13",
			Description:  "Debian 13 (Trixie) cloud image",
			Architecture: "amd64",
		},
		{
			ID:           "debian-11",
			Name:         "Debian 11 (Bullseye)",
			URL:          "https://cloud.debian.org/images/cloud/bullseye/20250801-2191/debian-11-generic-amd64-20250801-2191.qcow2",
			SizeGB:       1.9,
			Type:         "template",
			OS:           "Debian",
			Version:      "11",
			Description:  "Debian 11 (Bullseye) cloud image",
			Architecture: "amd64",
		},
		{
			ID:           "alpine-3.22",
			Name:         "Alpine Linux 3.22",
			URL:          "https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/cloud/generic_alpine-3.22.1-x86_64-bios-cloudinit-r0.qcow2",
			SizeGB:       0.5,
			Type:         "template",
			OS:           "Alpine",
			Version:      "3.22",
			Description:  "Alpine Linux 3.22 cloud image - minimal and secure",
			Architecture: "x86_64",
		},
		{
			ID:           "alpine-3.21",
			Name:         "Alpine Linux 3.21",
			URL:          "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/cloud/generic_alpine-3.21.4-x86_64-bios-cloudinit-r0.qcow2",
			SizeGB:       0.5,
			Type:         "template",
			OS:           "Alpine",
			Version:      "3.21",
			Description:  "Alpine Linux 3.21 cloud image - minimal and secure",
			Architecture: "x86_64",
		},
		{
			ID:           "alpine-3.20",
			Name:         "Alpine Linux 3.20",
			URL:          "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/cloud/generic_alpine-3.20.7-x86_64-bios-cloudinit-r0.qcow2",
			SizeGB:       0.5,
			Type:         "template",
			OS:           "Alpine",
			Version:      "3.20",
			Description:  "Alpine Linux 3.20 cloud image - minimal and secure",
			Architecture: "x86_64",
		},
		{
			ID:           "alpine-3.19",
			Name:         "Alpine Linux 3.19",
			URL:          "https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/cloud/nocloud_alpine-3.19.8-x86_64-bios-cloudinit-r0.qcow2",
			SizeGB:       0.5,
			Type:         "template",
			OS:           "Alpine",
			Version:      "3.19",
			Description:  "Alpine Linux 3.19 cloud image - minimal and secure",
			Architecture: "x86_64",
		},
		{
			ID:           "fedora-42",
			Name:         "Fedora 42",
			URL:          "https://download.fedoraproject.org/pub/fedora/linux/releases/42/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-42-1.1.x86_64.qcow2",
			SizeGB:       1.9,
			Type:         "template",
			OS:           "Fedora",
			Version:      "42",
			Description:  "Fedora 42 cloud base image",
			Architecture: "x86_64",
		},
		{
			ID:           "centos-stream-10",
			Name:         "CentOS Stream 10",
			URL:          "https://cloud.centos.org/centos/10-stream/x86_64/images/CentOS-Stream-GenericCloud-10-20250805.0.x86_64.qcow2",
			SizeGB:       1.6,
			Type:         "template",
			OS:           "CentOS",
			Version:      "Stream 10",
			Description:  "CentOS Stream 10 cloud image",
			Architecture: "x86_64",
		},
	}
}

// GetImages returns all available cloud images
func (r *ImageRepository) GetImages() []CloudImage {
	return r.Images
}

// GetImagesByOS returns images filtered by operating system
func (r *ImageRepository) GetImagesByOS(osName string) []CloudImage {
	var filtered []CloudImage
	for _, img := range r.Images {
		if strings.EqualFold(img.OS, osName) {
			filtered = append(filtered, img)
		}
	}
	return filtered
}

// DownloadImage downloads a cloud image with checksum verification
func (r *ImageRepository) DownloadImage(imageID string, progressCallback func(downloaded, total int64)) error {
	// Find the image
	var image *CloudImage
	for _, img := range r.Images {
		if img.ID == imageID {
			image = &img
			break
		}
	}
	if image == nil {
		return fmt.Errorf("image not found: %s", imageID)
	}

	// Create storage directory
	if err := os.MkdirAll(r.StoragePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Generate filename
	filename := fmt.Sprintf("%s.qcow2", image.ID)
	filepath := filepath.Join(r.StoragePath, filename)

	// Check if already downloaded
	if _, err := os.Stat(filepath); err == nil {
		return fmt.Errorf("image already exists: %s", filename)
	}

	// Download the image
	resp, err := http.Get(image.URL)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create output file
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Download with progress tracking
	var downloaded int64
	total := resp.ContentLength

	buffer := make([]byte, 32*1024) // 32KB buffer
	hash := sha256.New()

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			// Write to file
			if _, writeErr := out.Write(buffer[:n]); writeErr != nil {
				return fmt.Errorf("failed to write to file: %w", writeErr)
			}

			// Update hash
			hash.Write(buffer[:n])

			// Update progress
			downloaded += int64(n)
			if progressCallback != nil {
				progressCallback(downloaded, total)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("download error: %w", err)
		}
	}

	// Verify checksum if available
	if image.ChecksumURL != "" {
		if err := r.verifyChecksum(filepath, image.ChecksumURL, filename); err != nil {
			os.Remove(filepath) // Clean up on verification failure
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	return nil
}

// verifyChecksum downloads and verifies the SHA256 checksum
func (r *ImageRepository) verifyChecksum(filePath, checksumURL, filename string) error {
	// Download checksum file
	resp, err := http.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksum: %w", err)
	}
	defer resp.Body.Close()

	checksumData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read checksum: %w", err)
	}

	// Parse checksum file (format: "hash filename")
	lines := strings.Split(string(checksumData), "\n")
	var expectedHash string

	for _, line := range lines {
		if strings.Contains(line, filename) || strings.Contains(line, "cloudimg-amd64.img") {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				expectedHash = parts[0]
				break
			}
		}
	}

	if expectedHash == "" {
		return fmt.Errorf("checksum not found for file")
	}

	// Calculate actual hash
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for verification: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	actualHash := hex.EncodeToString(hash.Sum(nil))

	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// IsImageDownloaded checks if an image is already downloaded
func (r *ImageRepository) IsImageDownloaded(imageID string) bool {
	filename := fmt.Sprintf("%s.qcow2", imageID)
	filepath := filepath.Join(r.StoragePath, filename)
	_, err := os.Stat(filepath)
	return err == nil
}

// GetDownloadedImagePath returns the path to a downloaded image
func (r *ImageRepository) GetDownloadedImagePath(imageID string) string {
	filename := fmt.Sprintf("%s.qcow2", imageID)
	return filepath.Join(r.StoragePath, filename)
}
