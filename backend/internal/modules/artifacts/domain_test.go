package artifacts

import (
	"errors"
	"testing"
	"time"
)

const (
	testDigest      = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testChildDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestParseDigest(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "canonical", value: testDigest},
		{name: "uppercase", value: "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", wantErr: true},
		{name: "wrong algorithm", value: "sha512:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: true},
		{name: "short", value: "sha256:aaaa", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digest, err := ParseDigest(test.value)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidDigest) {
					t.Fatalf("ParseDigest() error = %v, want ErrInvalidDigest", err)
				}
				return
			}
			if err != nil || digest.String() != test.value {
				t.Fatalf("ParseDigest() = %q, %v", digest, err)
			}
		})
	}
}

func TestParseTagName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "lowercase", value: "latest"},
		{name: "case preserved", value: "Release_1.2-ARM64"},
		{name: "leading period", value: ".latest", wantErr: true},
		{name: "slash", value: "team/latest", wantErr: true},
		{name: "oversized", value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tag, err := ParseTagName(test.value)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidTag) {
					t.Fatalf("ParseTagName() error = %v, want ErrInvalidTag", err)
				}
				return
			}
			if err != nil || tag.String() != test.value {
				t.Fatalf("ParseTagName() = %q, %v", tag, err)
			}
		})
	}
}

func TestNormalizeObservation(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 123456789, time.FixedZone("test", 8*60*60))
	sourceCreatedAt := now.Add(-time.Hour)
	mediaType := "application/vnd.oci.image.index.v1+json"
	size := int64(512)
	tag := "Release_1"
	input := Observation{
		RepositoryID:    "repository-id",
		Digest:          testDigest,
		Kind:            "INDEX",
		MediaType:       &mediaType,
		SizeBytes:       &size,
		SourceCreatedAt: &sourceCreatedAt,
		Descriptors: &DescriptorSetObservation{Items: []DescriptorObservation{
			{
				Digest: testChildDigest,
				Platform: &Platform{
					OS: "linux", Architecture: "arm64", Variant: "v8",
				},
			},
		}},
		Tag:        &tag,
		ObservedAt: now,
	}
	reconciliation, err := NormalizeObservation(input)
	if err != nil {
		t.Fatalf("NormalizeObservation() error = %v", err)
	}
	if reconciliation.RepositoryID != input.RepositoryID ||
		reconciliation.Artifact.Digest.String() != testDigest ||
		reconciliation.Artifact.Kind != KindIndex ||
		reconciliation.ObservedAt.Location() != time.UTC ||
		!reconciliation.ObservedAt.Equal(now.UTC().Round(time.Microsecond)) ||
		reconciliation.Artifact.SourceCreatedAt == nil ||
		!reconciliation.Artifact.SourceCreatedAt.Equal(sourceCreatedAt.UTC().Round(time.Microsecond)) ||
		reconciliation.Tag == nil || reconciliation.Tag.String() != tag ||
		reconciliation.Descriptors == nil || len(reconciliation.Descriptors.Items) != 1 {
		t.Fatalf("reconciliation = %#v", reconciliation)
	}
}

func TestNormalizeObservationRejectsInvalidStructure(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	negative := int64(-1)
	tests := []struct {
		name  string
		input Observation
	}{
		{name: "missing repository", input: Observation{Digest: testDigest, Kind: "MANIFEST", ObservedAt: now}},
		{name: "missing time", input: Observation{RepositoryID: "repo", Digest: testDigest, Kind: "MANIFEST"}},
		{name: "invalid kind", input: Observation{RepositoryID: "repo", Digest: testDigest, Kind: "BLOB", ObservedAt: now}},
		{name: "negative size", input: Observation{RepositoryID: "repo", Digest: testDigest, Kind: "MANIFEST", SizeBytes: &negative, ObservedAt: now}},
		{name: "manifest descriptors", input: Observation{
			RepositoryID: "repo", Digest: testDigest, Kind: "MANIFEST", ObservedAt: now,
			Descriptors: &DescriptorSetObservation{},
		}},
		{name: "self reference", input: Observation{
			RepositoryID: "repo", Digest: testDigest, Kind: "INDEX", ObservedAt: now,
			Descriptors: &DescriptorSetObservation{Items: []DescriptorObservation{{Digest: testDigest}}},
		}},
		{name: "partial platform", input: Observation{
			RepositoryID: "repo", Digest: testDigest, Kind: "INDEX", ObservedAt: now,
			Descriptors: &DescriptorSetObservation{Items: []DescriptorObservation{{
				Digest: testChildDigest, Platform: &Platform{OS: "linux"},
			}}},
		}},
		{name: "platform control character", input: Observation{
			RepositoryID: "repo", Digest: testDigest, Kind: "INDEX", ObservedAt: now,
			Descriptors: &DescriptorSetObservation{Items: []DescriptorObservation{{
				Digest: testChildDigest, Platform: &Platform{OS: "lin\nux", Architecture: "amd64"},
			}}},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeObservation(test.input); !errors.Is(err, ErrInvalidArtifact) {
				t.Fatalf("NormalizeObservation() error = %v, want ErrInvalidArtifact", err)
			}
		})
	}
}

func TestNewPageRequest(t *testing.T) {
	request, err := NewArtifactPageRequest(100, testDigest)
	if err != nil || request.Limit != 100 || request.After != testDigest {
		t.Fatalf("NewArtifactPageRequest() = %#v, %v", request, err)
	}
	if _, err := NewArtifactPageRequest(0, ""); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("zero limit error = %v", err)
	}
	if _, err := NewArtifactPageRequest(101, ""); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("large limit error = %v", err)
	}
	if _, err := NewArtifactPageRequest(10, "invalid"); !errors.Is(err, ErrInvalidDigest) {
		t.Fatalf("invalid cursor error = %v", err)
	}
}
