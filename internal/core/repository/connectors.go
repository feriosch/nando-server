package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewConnectors returns the ConnectorsRepository over the connectors
// collection in db, first creating any indexes it lacks.
func NewConnectors(ctx context.Context, db *mongo.Database) (model.ConnectorsRepository, error) {
	collection := db.Collection("connectors")
	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "accessTokenHash", Value: 1}}},
		{Keys: bson.D{{Key: "refreshTokenHash", Value: 1}}},
		{Keys: bson.D{{Key: "retiredRefreshTokenHashes", Value: 1}}},
	})
	if err != nil {
		return nil, err
	}
	return connectors{collection: collection}, nil
}

// maxRetiredRefreshTokens is how many replaced refresh tokens a Connector
// remembers, to recognise one used again.
const maxRetiredRefreshTokens = 100

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

func (r connectors) FindByRefreshToken(ctx context.Context, hash []byte) (model.Connector, error) {
	return r.findOne(ctx, bson.D{{Key: "refreshTokenHash", Value: hash}})
}

func (r connectors) FindByRetiredRefreshToken(ctx context.Context, hash []byte) (model.Connector, error) {
	return r.findOne(ctx, bson.D{{Key: "retiredRefreshTokenHashes", Value: hash}})
}

func (r connectors) findOne(ctx context.Context, filter bson.D) (model.Connector, error) {
	var connector model.Connector
	err := r.collection.FindOne(ctx, filter).Decode(&connector)
	return connector, utils.NotFound(err)
}

func (r connectors) Rotate(ctx context.Context, id string, oldHash []byte, tokens model.ConnectorTokens) error {
	filter := bson.D{{Key: "_id", Value: id}, {Key: "refreshTokenHash", Value: oldHash}}
	update := bson.D{
		{Key: "$set", Value: tokens},
		{Key: "$push", Value: bson.D{{Key: "retiredRefreshTokenHashes", Value: bson.D{
			{Key: "$each", Value: bson.A{oldHash}},
			{Key: "$slice", Value: -maxRetiredRefreshTokens},
		}}}},
	}
	res, err := r.collection.UpdateOne(ctx, filter, update)
	return utils.Matched(res, err)
}

func (r connectors) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}

func (r connectors) List(ctx context.Context) ([]model.Connector, error) {
	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "approvedAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var list []model.Connector
	err = cursor.All(ctx, &list)
	return list, err
}

func (r connectors) MarkUsed(ctx context.Context, id string, at time.Time) error {
	_, err := r.collection.UpdateByID(ctx, id, bson.D{{Key: "$set", Value: bson.D{{Key: "lastUsedAt", Value: at}}}})
	return err
}
