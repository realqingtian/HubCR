// Command m2assert verifies event-driven Artifact state for the M2 full-stack test.
// It is not a product command.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/authorization"
	"hubcr.io/hubcr/internal/modules/repositories"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/artifactstore"
	"hubcr.io/hubcr/internal/platform/postgres/repositorystore"
)

const (
	organizationName  = "m2-e2e-team"
	publicRepository  = "public-image"
	privateRepository = "private-image"
	imageTag          = "smoke"
)

func main() {
	databaseConfig, err := config.LoadDatabase()
	if err != nil {
		fail("load database configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Options{
		URL: databaseConfig.URL, ConnectTimeout: databaseConfig.ConnectTimeout,
		MaxConnections: databaseConfig.MaxConnections,
	})
	if err != nil {
		fail("open PostgreSQL: %v", err)
	}
	defer pool.Close()

	repositoryService, err := repositories.NewService(
		repositorystore.New(pool.ORM()), authorization.NewPolicy(), time.Now,
	)
	if err != nil {
		fail("initialize repository service: %v", err)
	}
	artifactStore := artifactstore.New(pool.ORM())
	deadline := time.Now().Add(15 * time.Second)
	for {
		retry, err := verify(ctx, repositoryService, artifactStore)
		if err == nil {
			fmt.Println("event-driven Artifact metadata verified")
			return
		}
		if !retry || time.Now().After(deadline) {
			fail("verify event-driven Artifact metadata: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func verify(
	ctx context.Context,
	repositoryService *repositories.Service,
	artifactStore *artifactstore.Store,
) (bool, error) {
	publicRepositoryContext, err := repositoryService.AuthorizationContext(
		ctx, "", organizationName, publicRepository,
	)
	if err != nil {
		return retryable(err), err
	}
	privateRepositoryContext, err := repositoryService.AuthorizationContext(
		ctx, "", organizationName, privateRepository,
	)
	if err != nil {
		return retryable(err), err
	}
	publicTag, err := artifactStore.TagByName(
		ctx, publicRepositoryContext.Repository.ID, artifacts.TagName(imageTag),
	)
	if err != nil {
		return retryable(err), err
	}
	privateTag, err := artifactStore.TagByName(
		ctx, privateRepositoryContext.Repository.ID, artifacts.TagName(imageTag),
	)
	if err != nil {
		return retryable(err), err
	}
	if publicTag.Digest != privateTag.Digest {
		return false, errors.New("same pushed manifest produced different repository digests")
	}
	if publicTag.ArtifactID == privateTag.ArtifactID {
		return false, errors.New("repository-scoped Artifact IDs were physically deduplicated")
	}
	for _, input := range []struct {
		repositoryID string
		digest       artifacts.Digest
	}{
		{repositoryID: publicRepositoryContext.Repository.ID, digest: publicTag.Digest},
		{repositoryID: privateRepositoryContext.Repository.ID, digest: privateTag.Digest},
	} {
		artifact, err := artifactStore.ArtifactByDigest(ctx, input.repositoryID, input.digest)
		if err != nil {
			return retryable(err), err
		}
		if artifact.MediaType == nil || artifact.SizeBytes == nil || *artifact.SizeBytes <= 0 {
			return false, errors.New("event-driven Artifact metadata omitted Media Type or Size")
		}
		if artifact.Kind == artifacts.KindIndex && !artifact.DescriptorsComplete {
			return false, errors.New("event-driven Index descriptor set is incomplete")
		}
	}
	if _, err := artifactStore.TagByName(
		ctx, privateRepositoryContext.Repository.ID, artifacts.TagName("reader-denied"),
	); err == nil {
		return false, errors.New("denied push created Artifact tag metadata")
	} else if !errors.Is(err, artifacts.ErrNotFound) {
		return retryable(err), err
	}
	return false, nil
}

func retryable(err error) bool {
	return errors.Is(err, artifacts.ErrNotFound) || errors.Is(err, artifacts.ErrUnavailable) ||
		errors.Is(err, repositories.ErrNotFound)
}

func fail(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
