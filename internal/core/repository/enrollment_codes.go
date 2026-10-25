package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/model"
)

const enrollmentCodes = "enrollment_codes"

var enrollmentCodeIndexes = []mongo.IndexModel{expiresAtIndex}

func (r mongoRepository) InsertEnrollmentCode(ctx context.Context, code model.EnrollmentCode) error {
	_, err := r.db.Collection(enrollmentCodes).InsertOne(ctx, code)
	return err
}

func (r mongoRepository) FindEnrollmentCode(ctx context.Context, hash []byte, now time.Time) (model.EnrollmentCode, error) {
	var code model.EnrollmentCode
	err := r.db.Collection(enrollmentCodes).FindOne(ctx, bson.D{{Key: "_id", Value: hash}, unexpired(now)}).Decode(&code)
	return code, notFound(err)
}

func (r mongoRepository) UseEnrollmentCode(ctx context.Context, hash []byte, now time.Time) error {
	res, err := r.db.Collection(enrollmentCodes).DeleteOne(ctx, bson.D{{Key: "_id", Value: hash}, unexpired(now)})
	if err == nil && res.DeletedCount == 0 {
		return model.ErrNotFound
	}
	return err
}
