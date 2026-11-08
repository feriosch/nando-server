package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewPasskeys returns the PasskeysRepository over the passkeys collection in db.
func NewPasskeys(db *mongo.Database) model.PasskeysRepository {
	return passkeys{collection: db.Collection("passkeys")}
}

// passkeys is the passkeys collection.
type passkeys struct {
	collection *mongo.Collection
}

func (r passkeys) Insert(ctx context.Context, passkey model.Passkey) error {
	_, err := r.collection.InsertOne(ctx, passkey)
	return err
}

func (r passkeys) Find(ctx context.Context, id []byte) (model.Passkey, error) {
	var passkey model.Passkey
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&passkey)
	return passkey, utils.NotFound(err)
}

func (r passkeys) List(ctx context.Context) ([]model.Passkey, error) {
	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	list := []model.Passkey{}
	return list, cursor.All(ctx, &list)
}

func (r passkeys) Update(ctx context.Context, id []byte, old, credential []byte) error {
	return utils.Matched(r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "credential", Value: old}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "credential", Value: credential}}}},
	))
}

func (r passkeys) Delete(ctx context.Context, id []byte) error {
	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}
