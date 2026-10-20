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
	ctx := context.Background()

	container, err := mongodb.Run(ctx, "mongo:8")
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start MongoDB container (is Docker running?): %v", err)
	}

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("MongoDB container connection string: %v", err)
	}
	return uri
}
