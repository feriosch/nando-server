package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewAuthorizationCodes returns the AuthorizationCodesRepository over the
// authorization_codes collection in db, first creating any indexes it lacks.
func NewAuthorizationCodes(ctx context.Context, db *mongo.Database) (model.AuthorizationCodesRepository, error) {
	collection := db.Collection("authorization_codes")
	if _, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{utils.ExpiresAtIndex}); err != nil {
		return nil, err
	}
	return authorizationCodes{collection: collection}, nil
}

// authorizationCodes is the authorization_codes collection.
type authorizationCodes struct {
	collection *mongo.Collection
}

func (r authorizationCodes) Insert(ctx context.Context, code model.AuthorizationCode) error {
	_, err := r.collection.InsertOne(ctx, code)
	return err
}

func (r authorizationCodes) Take(ctx context.Context, hash []byte, now time.Time) (model.AuthorizationCode, error) {
	var code model.AuthorizationCode
	err := r.collection.FindOneAndDelete(ctx, bson.D{{Key: "_id", Value: hash}, utils.Unexpired(now)}).Decode(&code)
	return code, utils.NotFound(err)
}
