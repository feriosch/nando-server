// Package testmongo gives tests a real MongoDB. There are no Mongo mocks.
package testmongo

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
)

// URI starts a fresh, empty MongoDB container for this test and returns its
// connection string. The container is terminated when the test finishes.
// Docker must be running.
func URI(t *testing.T) string {
	t.Helper()
	uri, err := Start(t).ConnectionString(context.Background())
	if err != nil {
		t.Fatalf("MongoDB container connection string: %v", err)
	}
	return uri
}

// Start is URI for tests that need the container itself, such as to run
// mongodump in it, started with opts (such as mongodb.WithUsername).
func Start(t *testing.T, opts ...testcontainers.ContainerCustomizer) *mongodb.MongoDBContainer {
	t.Helper()
	container, err := mongodb.Run(context.Background(), "mongo:8", opts...)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start MongoDB container (is Docker running?): %v", err)
	}
	return container
}
