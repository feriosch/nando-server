package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
)

// NewGroups returns the GroupsRepository over the groups collection in db.
func NewGroups(db *mongo.Database) model.GroupsRepository {
	return groups{collection: db.Collection("groups")}
}

// groups is the groups collection.
type groups struct {
	collection *mongo.Collection
}

// groupDocument is a Group as it is stored. Its own "group" field, a category
// benkyo's screens bucketed Groups by, isn't read.
type groupDocument struct {
	Key      string `bson:"collection_name"`
	Name     string `bson:"printing_name"`
	ImageURL string `bson:"image_url"`
}

func (r groups) Find(ctx context.Context, key string) (model.Group, error) {
	var d groupDocument
	err := r.collection.FindOne(ctx, bson.D{{Key: "collection_name", Value: key}}).Decode(&d)
	return model.Group(d), utils.NotFound(err)
}
