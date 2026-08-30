package imagerepository

import "testing"

func TestDefaultImagesIncludeUbuntu2604LTS(t *testing.T) {
	const imageID = "ubuntu-26.04-lts"
	for _, image := range getDefaultImages() {
		if image.ID != imageID {
			continue
		}
		if image.Name != "Ubuntu 26.04 LTS (Resolute Raccoon)" {
			t.Fatalf("unexpected image name: %q", image.Name)
		}
		if image.URL != "https://cloud-images.ubuntu.com/releases/resolute/release/ubuntu-26.04-server-cloudimg-amd64.img" {
			t.Fatalf("unexpected image URL: %q", image.URL)
		}
		if image.ChecksumURL != "https://cloud-images.ubuntu.com/releases/resolute/release/SHA256SUMS" {
			t.Fatalf("unexpected checksum URL: %q", image.ChecksumURL)
		}
		if image.OS != "Ubuntu" || image.Version != "26.04 LTS" || image.Architecture != "amd64" {
			t.Fatalf("unexpected image metadata: %+v", image)
		}
		return
	}
	t.Fatalf("default image %q not found", imageID)
}
