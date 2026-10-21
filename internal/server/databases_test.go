package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/golobby/container/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/server"
	"github.com/feriosch/nando-server/internal/testmongo"
)

// newDatabases sets up Databases the way main does, disconnecting when the test
// ends.
func newDatabases(t *testing.T, ctx context.Context, uri string) (*server.Databases, error) {
	t.Helper()
	c := container.New()
	// Close whatever client the container holds, even if Fill below fails.
	t.Cleanup(func() {
		var client *mongo.Client
		if c.Resolve(&client) == nil {
			_ = client.Disconnect(context.Background())
		}
	})
	if err := server.RegisterDatabases(ctx, c, uri); err != nil {
		return nil, err
	}
	dbs := &server.Databases{}
	if err := c.Fill(dbs); err != nil {
		return nil, err
	}
	return dbs, nil
}

func TestCoreAndEachModuleGetTheirOwnDatabase(t *testing.T) {
	ctx := context.Background()
	dbs, err := newDatabases(t, ctx, testmongo.URI(t))
	if err != nil {
		t.Fatalf("newDatabases: %v", err)
	}

	for _, tc := range []struct {
		area string
		got  string
		want string
	}{
		{"Core", dbs.Core.Name(), "core"},
		{"Calories", dbs.Calories.Name(), "calories"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s database = %q, want %q", tc.area, tc.got, tc.want)
		}
	}
	if err := dbs.Core.Client().Ping(ctx, nil); err != nil {
		t.Errorf("ping through Core's database: %v", err)
	}
}

func TestUnreachableMongoIsAnError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Nothing listens on port 1.
	if _, err := newDatabases(t, ctx, "mongodb://127.0.0.1:1"); err == nil {
		t.Fatal("newDatabases on an unreachable MongoDB: err = nil, want an error")
	}
}
