package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewCompanies returns the CompaniesRepository over the companies collection
// in db.
func NewCompanies(db *mongo.Database) model.CompaniesRepository {
	return companies{collection: db.Collection("companies")}
}

// companies is the companies collection.
type companies struct {
	collection *mongo.Collection
}

func (r companies) Insert(ctx context.Context, company model.Company) (model.Company, error) {
	company.ID = bson.NewObjectID()
	_, err := r.collection.InsertOne(ctx, company)
	return company, err
}

func (r companies) GetByID(ctx context.Context, id bson.ObjectID) (model.Company, error) {
	var company model.Company
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&company)
	return company, utils.NotFound(err)
}

func (r companies) GetByName(ctx context.Context, name string) (model.Company, error) {
	var company model.Company
	err := r.collection.FindOne(ctx, bson.D{{Key: "name", Value: careerutils.Exactly(name)}}).Decode(&company)
	return company, utils.NotFound(err)
}

func (r companies) Search(ctx context.Context, query string) ([]model.Company, error) {
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "name", Value: careerutils.Containing(query)}}, careerutils.ByName())
	if err != nil {
		return nil, err
	}
	found := []model.Company{}
	return found, cursor.All(ctx, &found)
}

func (r companies) Replace(ctx context.Context, company model.Company) error {
	return utils.Matched(r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: company.ID}}, company))
}
