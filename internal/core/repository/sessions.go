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

// NewSessions returns the SessionsRepository over the sessions collection in
// db, first creating any indexes it lacks.
func NewSessions(ctx context.Context, db *mongo.Database) (model.SessionsRepository, error) {
	collection, err := expiringCollection(ctx, db, "sessions",
		mongo.IndexModel{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true)})
	if err != nil {
		return nil, err
	}
	return sessions{collection: collection}, nil
}

// sessions is the sessions collection.
type sessions struct {
	collection *mongo.Collection
}

func (r sessions) Insert(ctx context.Context, session model.Session) error {
	_, err := r.collection.InsertOne(ctx, session)
	return err
}

func (r sessions) Find(ctx context.Context, tokenHash []byte, now time.Time) (model.Session, error) {
	var session model.Session
	err := r.collection.FindOne(ctx, bson.D{{Key: "tokenHash", Value: tokenHash}, unexpired(now)}).Decode(&session)
	return session, utils.NotFound(err)
}

func (r sessions) List(ctx context.Context, now time.Time) ([]model.Session, error) {
	cursor, err := r.collection.Find(ctx, bson.D{unexpired(now)}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Session{}
	return list, cursor.All(ctx, &list)
}

func (r sessions) Renew(ctx context.Context, id string, renewedAt, expiresAt time.Time) error {
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "renewedAt", Value: renewedAt}, {Key: "expiresAt", Value: expiresAt}}}}
	_, err := r.collection.UpdateByID(ctx, id, update)
	return err
}

func (r sessions) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}
