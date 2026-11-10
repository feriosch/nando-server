package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewAuthorizationRequests returns the AuthorizationRequestsRepository over
// the authorization_requests collection in db, first creating any indexes it
// lacks.
func NewAuthorizationRequests(ctx context.Context, db *mongo.Database) (model.AuthorizationRequestsRepository, error) {
	collection := db.Collection("authorization_requests")
	if _, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{utils.ExpiresAtIndex}); err != nil {
		return nil, err
	}
	return authorizationRequests{collection: collection}, nil
}

// authorizationRequests is the authorization_requests collection.
type authorizationRequests struct {
	collection *mongo.Collection
}

func (r authorizationRequests) Insert(ctx context.Context, request model.AuthorizationRequest) error {
	_, err := r.collection.InsertOne(ctx, request)
	return err
}

func (r authorizationRequests) Find(ctx context.Context, id string, now time.Time) (model.AuthorizationRequest, error) {
	var request model.AuthorizationRequest
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}, utils.Unexpired(now)}).Decode(&request)
	return request, utils.NotFound(err)
}

func (r authorizationRequests) Take(ctx context.Context, id string, now time.Time) (model.AuthorizationRequest, error) {
	var request model.AuthorizationRequest
	err := r.collection.FindOneAndDelete(ctx, bson.D{{Key: "_id", Value: id}, utils.Unexpired(now)}).Decode(&request)
	return request, utils.NotFound(err)
}
