package artifacts

import (
	"context"
	"errors"
	"testing"
	"time"
)

const queryChildDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestQueryServiceReturnsArtifactDetailAndPreservesDescriptorKnowledge(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	reader := &queryReader{artifact: Artifact{
		ID: "artifact-id", RepositoryID: "repository-id", Digest: Digest(testDigest), Kind: KindIndex,
		DescriptorsComplete: true, DiscoveredAt: now, UpdatedAt: now,
	}, descriptors: []ManifestDescriptor{{
		Position: 0, ChildArtifactID: "child-id", Digest: Digest(queryChildDigest),
		Platform: &Platform{OS: "linux", Architecture: "arm64"},
	}}}
	service, err := NewQueryService(reader)
	if err != nil {
		t.Fatalf("NewQueryService() error = %v", err)
	}

	result, err := service.ArtifactDetail(context.Background(), "repository-id", testDigest)
	if err != nil {
		t.Fatalf("ArtifactDetail() error = %v", err)
	}
	if reader.descriptorCalls != 1 || len(result.Descriptors) != 1 ||
		result.Descriptors[0].Platform.Architecture != "arm64" {
		t.Fatalf("ArtifactDetail() = %#v, descriptor calls = %d", result, reader.descriptorCalls)
	}

	reader.artifact.DescriptorsComplete = false
	reader.descriptorCalls = 0
	result, err = service.ArtifactDetail(context.Background(), "repository-id", testDigest)
	if err != nil || result.Descriptors != nil || reader.descriptorCalls != 0 {
		t.Fatalf("unknown descriptors result/error/calls = %#v, %v, %d", result, err, reader.descriptorCalls)
	}

	reader.artifact.DescriptorsComplete = true
	reader.descriptors = nil
	result, err = service.ArtifactDetail(context.Background(), "repository-id", testDigest)
	if err != nil || result.Descriptors == nil || len(result.Descriptors) != 0 {
		t.Fatalf("known empty descriptors result/error = %#v, %v", result, err)
	}
}

func TestQueryServiceValidatesInputsBeforeReading(t *testing.T) {
	reader := &queryReader{}
	service, err := NewQueryService(reader)
	if err != nil {
		t.Fatalf("NewQueryService() error = %v", err)
	}

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{name: "artifact repository", call: func() error {
			_, err := service.ArtifactDetail(context.Background(), "", testDigest)
			return err
		}, want: ErrInvalidArtifact},
		{name: "artifact digest", call: func() error {
			_, err := service.ArtifactDetail(context.Background(), "repository-id", "invalid")
			return err
		}, want: ErrInvalidDigest},
		{name: "tag", call: func() error {
			_, err := service.TagDetail(context.Background(), "repository-id", "bad/tag")
			return err
		}, want: ErrInvalidTag},
		{name: "artifact page", call: func() error {
			_, err := service.ListArtifacts(context.Background(), "repository-id", 0, "")
			return err
		}, want: ErrInvalidArtifact},
		{name: "tag cursor", call: func() error {
			_, err := service.ListTags(context.Background(), "repository-id", 20, "bad/tag")
			return err
		}, want: ErrInvalidTag},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
	if reader.calls != 0 {
		t.Fatalf("reader calls = %d, want 0", reader.calls)
	}
}

func TestQueryServiceListsAndValidatesRepositoryScopedState(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	reader := &queryReader{
		artifactPage: ArtifactPage{Items: []Artifact{{
			ID: "artifact-id", RepositoryID: "repository-id", Digest: Digest(testDigest),
			Kind: KindManifest, DiscoveredAt: now, UpdatedAt: now,
		}}, NextAfter: testDigest},
		tag: Tag{
			RepositoryID: "repository-id", Name: TagName("latest"), ArtifactID: "artifact-id",
			Digest: Digest(testDigest), CreatedAt: now, UpdatedAt: now,
		},
		tagPage: TagPage{Items: []Tag{{
			RepositoryID: "repository-id", Name: TagName("latest"), ArtifactID: "artifact-id",
			Digest: Digest(testDigest), CreatedAt: now, UpdatedAt: now,
		}}, NextAfter: "latest"},
	}
	service, err := NewQueryService(reader)
	if err != nil {
		t.Fatalf("NewQueryService() error = %v", err)
	}

	artifactsPage, err := service.ListArtifacts(context.Background(), "repository-id", 1, "")
	if err != nil || artifactsPage.NextAfter != testDigest {
		t.Fatalf("ListArtifacts() = %#v, %v", artifactsPage, err)
	}
	tagsPage, err := service.ListTags(context.Background(), "repository-id", 1, "")
	if err != nil || tagsPage.NextAfter != "latest" {
		t.Fatalf("ListTags() = %#v, %v", tagsPage, err)
	}
	tag, err := service.TagDetail(context.Background(), "repository-id", "latest")
	if err != nil || tag.Digest.String() != testDigest {
		t.Fatalf("TagDetail() = %#v, %v", tag, err)
	}

	reader.artifactPage.Items[0].RepositoryID = "other-repository"
	if _, err := service.ListArtifacts(context.Background(), "repository-id", 1, ""); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("cross-repository ListArtifacts() error = %v", err)
	}
}

type queryReader struct {
	artifact        Artifact
	tag             Tag
	artifactPage    ArtifactPage
	tagPage         TagPage
	descriptors     []ManifestDescriptor
	err             error
	calls           int
	descriptorCalls int
}

func (r *queryReader) ArtifactByDigest(context.Context, string, Digest) (Artifact, error) {
	r.calls++
	return r.artifact, r.err
}

func (r *queryReader) TagByName(context.Context, string, TagName) (Tag, error) {
	r.calls++
	return r.tag, r.err
}

func (r *queryReader) ListArtifacts(context.Context, string, PageRequest) (ArtifactPage, error) {
	r.calls++
	return r.artifactPage, r.err
}

func (r *queryReader) ListTags(context.Context, string, PageRequest) (TagPage, error) {
	r.calls++
	return r.tagPage, r.err
}

func (r *queryReader) DescriptorsByIndex(context.Context, string, Digest) ([]ManifestDescriptor, error) {
	r.calls++
	r.descriptorCalls++
	return r.descriptors, r.err
}
