package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/japanese/model"
)

// NewIrregularComponents returns the IrregularComponentsRepository over the
// irregular_components collection in db.
func NewIrregularComponents(db *mongo.Database) model.IrregularComponentsRepository {
	return irregularComponents{collection: db.Collection("irregular_components")}
}

// irregularComponents is the irregular_components collection. Each document
// is {component, radicals}: the shape's keyword and benkyo's radicals for it.
type irregularComponents struct {
	collection *mongo.Collection
}

func (r irregularComponents) Stored(ctx context.Context, keywords []string) ([]string, error) {
	var stored []string
	err := r.collection.Distinct(ctx, "component", bson.D{{Key: "component", Value: bson.D{{Key: "$in", Value: keywords}}}}).Decode(&stored)
	return stored, err
}
