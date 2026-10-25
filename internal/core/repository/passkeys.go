package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
)

const passkeys = "passkeys"

func (r mongoRepository) InsertPasskey(ctx context.Context, passkey model.Passkey) error {
	_, err := r.db.Collection(passkeys).InsertOne(ctx, passkey)
	return err
}

func (r mongoRepository) FindPasskey(ctx context.Context, id []byte) (model.Passkey, error) {
	var passkey model.Passkey
	err := r.db.Collection(passkeys).FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&passkey)
	return passkey, notFound(err)
}

func (r mongoRepository) ListPasskeys(ctx context.Context) ([]model.Passkey, error) {
	cursor, err := r.db.Collection(passkeys).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Passkey{}
	return list, cursor.All(ctx, &list)
}

func (r mongoRepository) DeletePasskey(ctx context.Context, id []byte) error {
	_, err := r.db.Collection(passkeys).DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}
