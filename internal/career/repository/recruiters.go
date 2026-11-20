package repository

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

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

func (r recruiters) Search(ctx context.Context, query string) ([]model.Recruiter, error) {
	matches := bson.A{
		bson.D{{Key: "name", Value: careerutils.Containing(query)}},
		bson.D{{Key: "organisation", Value: careerutils.Containing(query)}},
	}
	// A profile link matches whole, with or without its trailing slash: its
	// id is case-sensitive. An empty one matches nobody.
	if link := strings.TrimSuffix(query, "/"); link != "" {
		matches = append(matches, bson.D{{Key: "profileUrl", Value: bson.D{{Key: "$in", Value: bson.A{link, link + "/"}}}}})
	}
	cursor, err := r.collection.Find(ctx, bson.D{{Key: "$or", Value: matches}}, careerutils.ByName())
	if err != nil {
		return nil, err
	}
	found := []model.Recruiter{}
	return found, cursor.All(ctx, &found)
}

func (r recruiters) FillProfileURL(ctx context.Context, id bson.ObjectID, url string) (bool, error) {
	result, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "profileUrl", Value: bson.D{{Key: "$in", Value: bson.A{"", nil}}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "profileUrl", Value: url}}}},
	)
	if err != nil {
		return false, err
	}
	return result.ModifiedCount == 1, nil
}
