package registry

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/go-version"
)

type ImageInfo struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

type RegistryClient struct {
	auth authn.Authenticator
}

func NewRegistryClient(username, password string) *RegistryClient {
	var auth authn.Authenticator
	if username != "" && password != "" {
		auth = &authn.Basic{
			Username: username,
			Password: password,
		}
	} else {
		auth = authn.Anonymous
	}
	return &RegistryClient{auth: auth}
}

// Parse image name into components
func ParseImage(image string) (*ImageInfo, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference: %v", err)
	}

	registry := ref.Context().Registry.Name()
	repository := ref.Context().RepositoryStr()

	var tag, digest string
	if tagRef, ok := ref.(name.Tag); ok {
		tag = tagRef.TagStr()
	} else if digestRef, ok := ref.(name.Digest); ok {
		digest = digestRef.DigestStr()
	}

	return &ImageInfo{
		Registry:   registry,
		Repository: repository,
		Tag:        tag,
		Digest:     digest,
	}, nil
}

// Get all available tags for an image
func (c *RegistryClient) ListTags(ctx context.Context, image string) ([]string, error) {
	imageInfo, err := ParseImage(image)
	if err != nil {
		return nil, err
	}

	repo, err := name.NewRepository(fmt.Sprintf("%s/%s", imageInfo.Registry, imageInfo.Repository))
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %v", err)
	}

	tags, err := remote.List(repo, remote.WithAuth(c.auth), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %v", err)
	}

	return tags, nil
}

// Get digest for a specific tag
func (c *RegistryClient) GetDigest(ctx context.Context, image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %v", err)
	}

	desc, err := remote.Get(ref, remote.WithAuth(c.auth), remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("failed to get image descriptor: %v", err)
	}

	return desc.Digest.String(), nil
}

// SortAlphabeticalTags sorts tags in descending lexicographical order.
func SortAlphabeticalTags(tags []string) []string {
	sort.Sort(sort.Reverse(sort.StringSlice(tags)))
	return tags
}

// Sort version tags (e.g., v1.2.3, 1.2.3)
func SortVersionTags(tags []string) []string {
	var versions []string
	var versionMap = make(map[string]*version.Version)

	for _, tag := range tags {
		// Remove 'v' prefix if exists
		cleanTag := strings.TrimPrefix(tag, "v")

		// Try to parse as version
		v, err := version.NewVersion(cleanTag)
		if err == nil {
			versions = append(versions, tag)
			versionMap[tag] = v
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		v1 := versionMap[versions[i]]
		v2 := versionMap[versions[j]]

		// If versions are equal, prefer the one without suffix
		if v1.Equal(v2) {
			// Get original tags
			t1 := versions[i]
			t2 := versions[j]
			// Remove 'v' prefix if exists
			t1 = strings.TrimPrefix(t1, "v")
			t2 = strings.TrimPrefix(t2, "v")
			// Check for suffixes
			hasSuffix1 := strings.Contains(t1, "-")
			hasSuffix2 := strings.Contains(t2, "-")
			if hasSuffix1 != hasSuffix2 {
				return !hasSuffix1 // Prefer the one without suffix
			}
			return t1 > t2 // If both have or don't have suffixes, use lexicographical order
		}

		return v1.GreaterThan(v2)
	})

	return versions
}

func parseInt(s string) (int, error) {
	var num int
	var err error
	fmt.Sscanf(s, "%d", &num)
	return num, err
}
