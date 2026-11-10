package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewOAuthClients returns the OAuthClientsRepository over the oauth_clients
// collection in db, first creating any indexes it lacks.
func NewOAuthClients(ctx context.Context, db *mongo.Database) (model.OAuthClientsRepository, error) {
	collection := db.Collection("oauth_clients")
	if _, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{utils.ExpiresAtIndex}); err != nil {
		return nil, err
	}
	return oauthClients{collection: collection}, nil
}

// oauthClients is the oauth_clients collection.
type oauthClients struct {
	collection *mongo.Collection
}

func (r oauthClients) Insert(ctx context.Context, client model.OAuthClient) error {
	_, err := r.collection.InsertOne(ctx, client)
	return err
}

func (r oauthClients) Find(ctx context.Context, id string, now time.Time) (model.OAuthClient, error) {
	var client model.OAuthClient
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}, utils.Unexpired(now)}).Decode(&client)
	return client, utils.NotFound(err)
}
