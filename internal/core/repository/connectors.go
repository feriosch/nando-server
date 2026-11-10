package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewConnectors returns the ConnectorsRepository over the connectors
// collection in db.
func NewConnectors(db *mongo.Database) model.ConnectorsRepository {
	return connectors{collection: db.Collection("connectors")}
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
