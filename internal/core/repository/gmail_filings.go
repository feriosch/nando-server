package repository

import (
	"context"
	"regexp"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// NewGmailFilings returns the GmailFilingsRepository over the gmail_filings
// collection in db, creating its index.
func NewGmailFilings(ctx context.Context, db *mongo.Database) (model.GmailFilingsRepository, error) {
	collection := db.Collection("gmail_filings")
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "day", Value: 1}, {Key: "at", Value: 1}},
	})
	return gmailFilings{collection: collection}, err
}

// gmailFilings is the gmail_filings collection.
type gmailFilings struct {
	collection *mongo.Collection
}

func (r gmailFilings) Add(ctx context.Context, filings []model.GmailFiling) error {
	if len(filings) == 0 {
		return nil
	}
	_, err := r.collection.InsertMany(ctx, filings)
	return err
}

func (r gmailFilings) List(ctx context.Context, filter model.GmailFilingsFilter, limit int) ([]model.GmailFiling, bool, error) {
	query := bson.D{{Key: "day", Value: bson.D{{Key: "$gte", Value: filter.From}, {Key: "$lte", Value: filter.To}}}}
	if filter.Query != "" {
		pattern := bson.Regex{Pattern: regexp.QuoteMeta(filter.Query), Options: "i"}
		query = append(query, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "subject", Value: pattern}},
			bson.D{{Key: "from", Value: pattern}},
			bson.D{{Key: "reason", Value: pattern}},
		}})
	}
	// The newest limit+1, to tell whether there are more than limit.
	cursor, err := r.collection.Find(ctx, query, options.Find().
		SetSort(bson.D{{Key: "day", Value: -1}, {Key: "at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit+1)))
	if err != nil {
		return nil, false, err
	}
	list := []model.GmailFiling{}
	if err := cursor.All(ctx, &list); err != nil {
		return nil, false, err
	}
	more := len(list) > limit
	if more {
		list = list[:limit]
	}
	slices.Reverse(list)
	return list, more, nil
}

func (r gmailFilings) MarkUndone(ctx context.Context, id string, at time.Time) (model.GmailFiling, error) {
	_, err := r.collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "undoneAt", Value: nil}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "undoneAt", Value: at}}}})
	if err != nil {
		return model.GmailFiling{}, err
	}
	var filing model.GmailFiling
	err = r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&filing)
	return filing, utils.NotFound(err)
}

func (r gmailFilings) DeleteBefore(ctx context.Context, t time.Time) error {
	_, err := r.collection.DeleteMany(ctx, bson.D{{Key: "at", Value: bson.D{{Key: "$lt", Value: t}}}})
	return err
}
