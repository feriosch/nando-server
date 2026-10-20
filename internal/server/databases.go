package server

import (
	"context"
	"errors"
	"time"

	"github.com/golobby/container/v3"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Databases is the connection to MongoDB, with one database for Core and one
// for each Module. Core and each Module are handed only their own database.
// Fill it from a container set up by RegisterDatabases.
type Databases struct {
	client   *mongo.Client   `container:"type"`
	Core     *mongo.Database `container:"name"`
	Calories *mongo.Database `container:"name"`
}

// RegisterDatabases connects to the MongoDB at uri and binds singletons in c:
// the *mongo.Client, and a *mongo.Database named after each field of Databases.
func RegisterDatabases(ctx context.Context, c container.Container, uri string) error {
	err := c.Singleton(func() (*mongo.Client, error) {
		client, err := mongo.Connect(options.Client().ApplyURI(uri))
		if err != nil {
			return nil, err
		}
		if err := client.Ping(ctx, nil); err != nil {
			// ctx may have expired, so close with a context of our own.
			closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return nil, errors.Join(err, client.Disconnect(closeCtx))
		}
		return client, nil
	})
	if err != nil {
		return err
	}

	for name, database := range map[string]string{"Core": "core", "Calories": "calories"} {
		err := c.NamedSingleton(name, func(client *mongo.Client) *mongo.Database {
			return client.Database(database)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Disconnect closes the connection, if one was filled in.
func (d *Databases) Disconnect(ctx context.Context) error {
	if d.client == nil {
		return nil
	}
	return d.client.Disconnect(ctx)
}
