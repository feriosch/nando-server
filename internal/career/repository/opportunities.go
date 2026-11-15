package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewOpportunities returns the OpportunitiesRepository over the
// opportunities collection in db.
func NewOpportunities(db *mongo.Database) model.OpportunitiesRepository {
	return opportunities{collection: db.Collection("opportunities")}
}

// opportunities is the opportunities collection.
type opportunities struct {
	collection *mongo.Collection
}

func (r opportunities) Insert(ctx context.Context, opportunity model.Opportunity) (model.Opportunity, error) {
	opportunity.ID = bson.NewObjectID()
	_, err := r.collection.InsertOne(ctx, opportunity)
	return opportunity, err
}

func (r opportunities) GetByID(ctx context.Context, id bson.ObjectID) (model.Opportunity, error) {
	var opportunity model.Opportunity
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&opportunity)
	return opportunity, utils.NotFound(err)
}

func (r opportunities) Replace(ctx context.Context, opportunity model.Opportunity) error {
	return utils.Matched(r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: opportunity.ID}}, opportunity))
}

func (r opportunities) Delete(ctx context.Context, id bson.ObjectID) error {
	return utils.Deleted(r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}}))
}

func (r opportunities) ByRecruiter(ctx context.Context, recruiterID bson.ObjectID) ([]model.Opportunity, error) {
	return r.find(ctx, bson.D{{Key: "recruiterId", Value: recruiterID}})
}

func (r opportunities) ByCompany(ctx context.Context, companyID bson.ObjectID) ([]model.Opportunity, error) {
	return r.find(ctx, bson.D{{Key: "companyId", Value: companyID}})
}

func (r opportunities) Search(ctx context.Context, query string, companyIDs []bson.ObjectID) ([]model.Opportunity, error) {
	return r.find(ctx, bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "role", Value: careerutils.Containing(query)}},
		bson.D{{Key: "companyId", Value: bson.D{{Key: "$in", Value: companyIDs}}}},
	}}})
}

func (r opportunities) Open(ctx context.Context) ([]model.Opportunity, error) {
	return r.find(ctx, bson.D{{Key: "stage", Value: bson.D{{Key: "$ne", Value: model.StageClosed}}}})
}

// find returns the Opportunities matching filter, oldest first.
func (r opportunities) find(ctx context.Context, filter bson.D) ([]model.Opportunity, error) {
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	found := []model.Opportunity{}
	return found, cursor.All(ctx, &found)
}
