package repository

import (
	"context"
	"errors"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
)

// NewStatements returns the StatementsRepository over the statements
// collection in db, with a unique index on Account and period: one Statement
// per Account and period.
func NewStatements(ctx context.Context, db *mongo.Database) (model.StatementsRepository, error) {
	collection := db.Collection("statements")
	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{{
		Keys:    bson.D{{Key: "accountId", Value: 1}, {Key: "from", Value: 1}, {Key: "to", Value: 1}},
		Options: options.Index().SetUnique(true),
	}, {
		Keys: bson.D{{Key: "transactions.id", Value: 1}},
	}})
	return statements{collection: collection}, err
}

// statements is the statements collection.
type statements struct {
	collection *mongo.Collection
}

// withoutTransactions leaves a Statement's Transactions out of what is read.
var withoutTransactions = bson.D{{Key: "transactions", Value: 0}}

func (r statements) Save(ctx context.Context, s model.Statement) (model.Statement, error) {
	period := bson.D{{Key: "accountId", Value: s.AccountID}, {Key: "from", Value: s.From}, {Key: "to", Value: s.To}}
	var old model.Statement
	err := r.collection.FindOne(ctx, period, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}})).Decode(&old)
	switch {
	case err == nil:
		s.ID = old.ID
	case errors.Is(err, mongo.ErrNoDocuments):
		s.ID = bson.NewObjectID().Hex()
	default:
		return s, err
	}
	for i := range s.Transactions {
		s.Transactions[i].ID = bson.NewObjectID().Hex()
	}
	_, err = r.collection.ReplaceOne(ctx, period, s, options.Replace().SetUpsert(true))
	return s, err
}

func (r statements) Find(ctx context.Context, id string) (model.Statement, error) {
	var s model.Statement
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&s)
	return s, utils.NotFound(err)
}

func (r statements) ForAccount(ctx context.Context, accountID, fromDay, toDay string) ([]model.Statement, error) {
	filter := bson.D{{Key: "accountId", Value: accountID}}
	if fromDay != "" {
		filter = append(filter, bson.E{Key: "to", Value: bson.D{{Key: "$gte", Value: fromDay}}})
	}
	if toDay != "" {
		filter = append(filter, bson.E{Key: "from", Value: bson.D{{Key: "$lte", Value: toDay}}})
	}
	opts := options.Find().SetSort(bson.D{{Key: "from", Value: 1}}).SetProjection(withoutTransactions)
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	list := []model.Statement{}
	return list, cursor.All(ctx, &list)
}

func (r statements) Delete(ctx context.Context, id string) error {
	result, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err == nil && result.DeletedCount == 0 {
		return utils.ErrNotFound
	}
	return err
}

func (r statements) AccountHasStatements(ctx context.Context, accountID string) (bool, error) {
	n, err := r.collection.CountDocuments(ctx, bson.D{{Key: "accountId", Value: accountID}}, options.Count().SetLimit(1))
	return n > 0, err
}

func (r statements) HasTransaction(ctx context.Context, accountID, exceptStatementID, day string, amount int64, rawText string) (bool, error) {
	n, err := r.collection.CountDocuments(ctx, bson.D{
		{Key: "accountId", Value: accountID},
		{Key: "_id", Value: bson.D{{Key: "$ne", Value: exceptStatementID}}},
		{Key: "transactions", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
			{Key: "day", Value: day}, {Key: "amount", Value: amount}, {Key: "rawText", Value: rawText},
			{Key: "duplicate", Value: bson.D{{Key: "$ne", Value: true}}},
		}}}},
	}, options.Count().SetLimit(1))
	return n > 0, err
}

func (r statements) Transactions(ctx context.Context, f model.TransactionFilter) ([]model.FoundTransaction, error) {
	var match bson.D
	if f.AccountID != "" {
		match = append(match, bson.E{Key: "accountId", Value: f.AccountID})
	}
	lines := bson.D{{Key: "transaction.duplicate", Value: bson.D{{Key: "$ne", Value: true}}}}
	if f.FromMonth != "" {
		lines = append(lines, bson.E{Key: "transaction.day", Value: bson.D{{Key: "$gte", Value: f.FromMonth + "-01"}}})
	}
	if f.ToMonth != "" {
		// "~" sorts after every Day of the month.
		lines = append(lines, bson.E{Key: "transaction.day", Value: bson.D{{Key: "$lte", Value: f.ToMonth + "-~"}}})
	}
	if f.Kind != "" {
		lines = append(lines, bson.E{Key: "transaction.kind", Value: f.Kind})
	}
	for key, value := range map[string]string{
		"transaction.category": f.Category, "transaction.subcategoryId": f.SubcategoryID, "transaction.merchantId": f.MerchantID,
	} {
		if value != "" {
			lines = append(lines, bson.E{Key: key, Value: value})
		}
	}
	if f.Unconfirmed != nil {
		if *f.Unconfirmed {
			lines = append(lines, bson.E{Key: "transaction.unconfirmed", Value: true})
		} else {
			lines = append(lines, bson.E{Key: "transaction.unconfirmed", Value: bson.D{{Key: "$ne", Value: true}}})
		}
	}
	if f.OneOff != nil {
		if *f.OneOff {
			lines = append(lines, bson.E{Key: "transaction.oneOff", Value: true})
		} else {
			lines = append(lines, bson.E{Key: "transaction.oneOff", Value: bson.D{{Key: "$ne", Value: true}}})
		}
	}
	if f.Text != "" {
		word := bson.Regex{Pattern: regexp.QuoteMeta(f.Text), Options: "i"}
		lines = append(lines, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "transaction.rawText", Value: word}}, bson.D{{Key: "transaction.note", Value: word}},
		}})
	}
	pipeline := mongo.Pipeline{}
	if len(match) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: match}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$unwind", Value: "$transactions"}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0}, {Key: "transaction", Value: "$transactions"},
			{Key: "statementId", Value: "$_id"}, {Key: "accountId", Value: 1},
		}}},
	)
	pipeline = append(pipeline, bson.D{{Key: "$match", Value: lines}})
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "transaction.day", Value: 1}, {Key: "transaction.id", Value: 1}}}})
	if f.Limit > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$limit", Value: f.Limit}})
	}
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	list := []model.FoundTransaction{}
	return list, cursor.All(ctx, &list)
}

func (r statements) Duplicates(ctx context.Context, accountID, exceptStatementID string) ([]model.FoundTransaction, error) {
	cursor, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "accountId", Value: accountID},
			{Key: "_id", Value: bson.D{{Key: "$ne", Value: exceptStatementID}}},
			{Key: "transactions.duplicate", Value: true},
		}}},
		bson.D{{Key: "$unwind", Value: "$transactions"}},
		bson.D{{Key: "$match", Value: bson.D{{Key: "transactions.duplicate", Value: true}}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0}, {Key: "transaction", Value: "$transactions"},
			{Key: "statementId", Value: "$_id"}, {Key: "accountId", Value: 1},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "statementId", Value: 1}}}},
	})
	if err != nil {
		return nil, err
	}
	list := []model.FoundTransaction{}
	return list, cursor.All(ctx, &list)
}

func (r statements) Count(ctx context.Context, id string) error {
	_, err := r.collection.UpdateOne(ctx, bson.D{{Key: "transactions.id", Value: id}},
		bson.D{{Key: "$unset", Value: bson.D{{Key: "transactions.$.duplicate", Value: ""}}}})
	return err
}

func (r statements) MoveSubcategory(ctx context.Context, from, into, intoCategory string) error {
	_, err := r.collection.UpdateMany(ctx, bson.D{{Key: "transactions.subcategoryId", Value: from}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "transactions.$[t].subcategoryId", Value: into}, {Key: "transactions.$[t].category", Value: intoCategory},
		}}},
		options.UpdateMany().SetArrayFilters([]any{bson.D{{Key: "t.subcategoryId", Value: from}}}))
	return err
}

// errorsIsNoDocuments is whether err is the driver's "no documents".
func errorsIsNoDocuments(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}

func (r statements) FindTransaction(ctx context.Context, id string) (model.FoundTransaction, error) {
	var s model.Statement
	err := r.collection.FindOne(ctx, bson.D{{Key: "transactions.id", Value: id}}).Decode(&s)
	if err != nil {
		return model.FoundTransaction{}, utils.NotFound(err)
	}
	for _, t := range s.Transactions {
		if t.ID == id {
			return model.FoundTransaction{Transaction: t, StatementID: s.ID, AccountID: s.AccountID}, nil
		}
	}
	return model.FoundTransaction{}, utils.ErrNotFound
}

func (r statements) UpdateTransaction(ctx context.Context, t model.Transaction) error {
	result, err := r.collection.UpdateOne(ctx, bson.D{{Key: "transactions.id", Value: t.ID}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "transactions.$", Value: t}}}})
	if err == nil && result.MatchedCount == 0 {
		return utils.ErrNotFound
	}
	return err
}
