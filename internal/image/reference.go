package image

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/alekpopovic/orch/pkg/types"
)

var digestPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+.-]*:[a-fA-F0-9]{32,}$`)

type Resolver interface {
	Resolve(context.Context, string) (types.ImageMetadata, error)
}

type ParserResolver struct{}

func (ParserResolver) Resolve(_ context.Context, reference string) (types.ImageMetadata, error) {
	return Parse(reference)
}

func Parse(reference string) (types.ImageMetadata, error) {
	requested := strings.TrimSpace(reference)
	if requested == "" {
		return types.ImageMetadata{}, fmt.Errorf("image reference is required")
	}

	nameAndTag := requested
	digest := ""
	if before, after, found := strings.Cut(requested, "@"); found {
		if strings.Contains(after, "@") || !digestPattern.MatchString(after) {
			return types.ImageMetadata{}, fmt.Errorf("image digest is invalid")
		}
		nameAndTag = before
		digest = strings.ToLower(after)
	}

	lastSlash := strings.LastIndexByte(nameAndTag, '/')
	lastColon := strings.LastIndexByte(nameAndTag, ':')
	tag := ""
	namePart := nameAndTag
	if lastColon > lastSlash {
		tag = nameAndTag[lastColon+1:]
		namePart = nameAndTag[:lastColon]
		if tag == "" {
			return types.ImageMetadata{}, fmt.Errorf("image tag is empty")
		}
	}

	first, rest, hasSlash := strings.Cut(namePart, "/")
	registry := "docker.io"
	name := namePart
	if hasSlash && (strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost") {
		registry = strings.ToLower(first)
		name = rest
	}
	if registry == "docker.io" && !strings.Contains(name, "/") {
		name = "library/" + name
	}
	if name == "" || strings.ContainsAny(name, " @") {
		return types.ImageMetadata{}, fmt.Errorf("image name is invalid")
	}
	if tag == "" && digest == "" {
		tag = "latest"
	}

	return types.ImageMetadata{
		RequestedImage: requested,
		Digest:         digest,
		Registry:       registry,
		Name:           name,
		Tag:            tag,
	}, nil
}

func PinnedReference(metadata types.ImageMetadata) string {
	if strings.TrimSpace(metadata.Digest) == "" {
		return strings.TrimSpace(metadata.RequestedImage)
	}
	return strings.TrimSuffix(metadata.Registry, "/") + "/" + strings.TrimPrefix(metadata.Name, "/") + "@" + metadata.Digest
}

func IsMutable(metadata types.ImageMetadata) bool {
	return strings.TrimSpace(metadata.Digest) == ""
}
