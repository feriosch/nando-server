package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewConnectors returns the ConnectorsRepository over the connectors
// collection in db, first creating any indexes it lacks.
func NewConnectors(ctx context.Context, db *mongo.Database) (model.ConnectorsRepository, error) {
	collection := db.Collection("connectors")
	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "accessTokenHash", Value: 1}}},
	})
	if err != nil {
		return nil, err
	}
	return connectors{collection: collection}, nil
}

// connectors is the connectors collection.
type connectors struct {
	collection *mongo.Collection
}

func (r connectors) Insert(ctx context.Context, connector model.Connector) error {
	_, err := r.collection.InsertOne(ctx, connector)
	return err
}

func (r connectors) SetTokens(ctx context.Context, id string, tokens model.ConnectorTokens) error {
	res, err := r.collection.UpdateByID(ctx, id, bson.D{{Key: "$set", Value: tokens}})
	return utils.Matched(res, err)
}

func (r connectors) FindByAccessToken(ctx context.Context, hash []byte, now time.Time) (model.Connector, error) {
	var connector model.Connector
	filter := bson.D{
		{Key: "accessTokenHash", Value: hash},
		{Key: "accessExpiresAt", Value: bson.D{{Key: "$gt", Value: now}}},
	}
	err := r.collection.FindOne(ctx, filter).Decode(&connector)
	return connector, utils.NotFound(err)
}
