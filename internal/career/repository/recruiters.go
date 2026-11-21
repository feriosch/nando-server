package repository

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewRecruiters returns the RecruitersRepository over the recruiters
// collection in db.
func NewRecruiters(db *mongo.Database) model.RecruitersRepository {
	return recruiters{collection: db.Collection("recruiters")}
}

// recruiters is the recruiters collection.
type recruiters struct {
	collection *mongo.Collection
}

func (r recruiters) Insert(ctx context.Context, recruiter model.Recruiter) (model.Recruiter, error) {
	recruiter.ID = bson.NewObjectID()
	_, err := r.collection.InsertOne(ctx, recruiter)
	return recruiter, err
}

func (r recruiters) GetByID(ctx context.Context, id bson.ObjectID) (model.Recruiter, error) {
	var recruiter model.Recruiter
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&recruiter)
	return recruiter, utils.NotFound(err)
}

func (r recruiters) GiveProfileURL(ctx context.Context, id bson.ObjectID, url string) error {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "profileUrl", Value: bson.D{{Key: "$in", Value: bson.A{"", nil}}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "profileUrl", Value: url}}}},
	)
	return err
}

func (r recruiters) AddEmail(ctx context.Context, id bson.ObjectID, email string) error {
	// An address is one Recruiter's: the latest filing takes it from any
	// other, so ByEmail never has two to choose from.
	if _, err := r.collection.UpdateMany(ctx,
		bson.D{{Key: "_id", Value: bson.D{{Key: "$ne", Value: id}}}, {Key: "emails", Value: email}},
		bson.D{{Key: "$pull", Value: bson.D{{Key: "emails", Value: email}}}}); err != nil {
		return err
	}
	_, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$addToSet", Value: bson.D{{Key: "emails", Value: email}}}})
	return err
}

func (r recruiters) RemoveEmail(ctx context.Context, id bson.ObjectID, email string) (model.Recruiter, error) {
	var recruiter model.Recruiter
	err := r.collection.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$pull", Value: bson.D{{Key: "emails", Value: email}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&recruiter)
	return recruiter, utils.NotFound(err)
}

func (r recruiters) ByEmail(ctx context.Context, email string) (model.Recruiter, error) {
	var recruiter model.Recruiter
	err := r.collection.FindOne(ctx, bson.D{{Key: "emails", Value: email}}).Decode(&recruiter)
	return recruiter, utils.NotFound(err)
}

func (r recruiters) Search(ctx context.Context, query string) ([]model.Recruiter, error) {
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "name", Value: careerutils.Containing(query)}},
		bson.D{{Key: "organisation", Value: careerutils.Containing(query)}},
		bson.D{{Key: "profileUrl", Value: query}},
		bson.D{{Key: "emails", Value: strings.ToLower(query)}},
	}}}, careerutils.ByName())
	if err != nil {
		return nil, err
	}
	found := []model.Recruiter{}
	return found, cursor.All(ctx, &found)
}
