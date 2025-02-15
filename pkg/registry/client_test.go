package registry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test for ParseImage function
func TestParseImage(t *testing.T) {
	tests := []struct {
		image   string
		wantErr bool
	}{
		{"docker.io/library/nginx:latest", false},
		{"gcr.io/my-project/my-image:v1.0.0", false},
		{"registry.invalid/image@sha256:1234567890abcdef", false},
		{"invalid-image-reference", true},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			_, err := ParseImage(tt.image)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got one")
			}
		})
	}
}

// Test for ListTags function
func TestListTags(t *testing.T) {
	ctx := context.Background()
	client := NewRegistryClient("", "") // 使用匿名访问

	// 使用公共镜像进行测试（如 Docker Hub 的 library/nginx）
	image := "docker.io/library/nginx"

	tags, err := client.ListTags(ctx, image)
	assert.NoError(t, err, "Failed to list tags")
	assert.NotEmpty(t, tags, "Expected tags but got empty list")

	t.Logf("Tags for %s: %v", image, tags)
}

// Test for GetDigest function
func TestGetDigest(t *testing.T) {
	ctx := context.Background()
	client := NewRegistryClient("", "") // 使用匿名访问

	image := "docker.io/library/nginx:latest"

	digest, err := client.GetDigest(ctx, image)
	assert.NoError(t, err, "Failed to get digest")
	assert.NotEmpty(t, digest, "Expected a digest but got empty")

	t.Logf("Digest for %s: %s", image, digest)
}

// Test for SortVersionTags function
func TestSortVersionTags(t *testing.T) {
	tags := []string{"v1.2.0", "v1.1.0", "v1.10.0", "v2.0.0", "1.5.0", "1.4.1"}
	expected := []string{"v2.0.0", "v1.10.0", "v1.2.0", "v1.1.0", "1.5.0", "1.4.1"}

	sortedTags := SortVersionTags(tags)
	assert.Equal(t, expected, sortedTags, "Version tags are not sorted correctly")

	t.Logf("Sorted Tags: %v", sortedTags)
}
