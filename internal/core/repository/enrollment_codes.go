package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewEnrollmentCodes returns the EnrollmentCodesRepository over the
// enrollment_codes collection in db, first creating any indexes it lacks.
func NewEnrollmentCodes(ctx context.Context, db *mongo.Database) (model.EnrollmentCodesRepository, error) {
	collection := db.Collection("enrollment_codes")
	if _, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{utils.ExpiresAtIndex}); err != nil {
		return nil, err
	}
	return enrollmentCodes{collection: collection}, nil
}

// enrollmentCodes is the enrollment_codes collection.
type enrollmentCodes struct {
	collection *mongo.Collection
}

func (r enrollmentCodes) Insert(ctx context.Context, code model.EnrollmentCode) error {
	_, err := r.collection.InsertOne(ctx, code)
	return err
}

func (r enrollmentCodes) Find(ctx context.Context, hash []byte, now time.Time) (model.EnrollmentCode, error) {
	var code model.EnrollmentCode
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: hash}, utils.Unexpired(now)}).Decode(&code)
	return code, utils.NotFound(err)
}

func (r enrollmentCodes) Use(ctx context.Context, hash []byte, now time.Time) error {
	return utils.Deleted(r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: hash}, utils.Unexpired(now)}))
}
