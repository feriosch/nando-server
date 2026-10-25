package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

const sessions = "sessions"

var sessionIndexes = []mongo.IndexModel{
	expiresAtIndex,
	{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true)},
}

func (r mongoRepository) InsertSession(ctx context.Context, session model.Session) error {
	_, err := r.db.Collection(sessions).InsertOne(ctx, session)
	return err
}

func (r mongoRepository) FindSession(ctx context.Context, tokenHash []byte, now time.Time) (model.Session, error) {
	var session model.Session
	err := r.db.Collection(sessions).FindOne(ctx, bson.D{{Key: "tokenHash", Value: tokenHash}, unexpired(now)}).Decode(&session)
	return session, notFound(err)
}

func (r mongoRepository) ListSessions(ctx context.Context, now time.Time) ([]model.Session, error) {
	cursor, err := r.db.Collection(sessions).Find(ctx, bson.D{unexpired(now)}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Session{}
	return list, cursor.All(ctx, &list)
}

func (r mongoRepository) RenewSession(ctx context.Context, id string, renewedAt, expiresAt time.Time) error {
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "renewedAt", Value: renewedAt}, {Key: "expiresAt", Value: expiresAt}}}}
	_, err := r.db.Collection(sessions).UpdateByID(ctx, id, update)
	return err
}

func (r mongoRepository) DeleteSession(ctx context.Context, id string) error {
	_, err := r.db.Collection(sessions).DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}
