package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/career/model"
)

// profileID is the _id of the only profile document.
const profileID = "profile"

// NewProfile returns the ProfileRepository over the profile collection in db.
func NewProfile(db *mongo.Database) model.ProfileRepository {
	return profile{collection: db.Collection("profile")}
}

// profile is the profile collection, with one document holding the Career
// Profile's Markdown.
type profile struct {
	collection *mongo.Collection
}

func (r profile) Find(ctx context.Context) (string, error) {
	var doc struct {
		Markdown string `bson:"markdown"`
	}
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: profileID}}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", nil
	}
	return doc.Markdown, err
}

func (r profile) Save(ctx context.Context, markdown string) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: profileID}},
		bson.D{{Key: "markdown", Value: markdown}}, options.Replace().SetUpsert(true))
	return err
}
