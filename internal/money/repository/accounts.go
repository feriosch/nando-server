package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
)

// NewAccounts returns the AccountsRepository over the accounts collection in db.
func NewAccounts(db *mongo.Database) model.AccountsRepository {
	return accounts{collection: db.Collection("accounts")}
}

// accounts is the accounts collection.
type accounts struct {
	collection *mongo.Collection
}

func (r accounts) Insert(ctx context.Context, account model.Account) (string, error) {
	account.ID = bson.NewObjectID().Hex()
	_, err := r.collection.InsertOne(ctx, account)
	return account.ID, err
}

func (r accounts) Find(ctx context.Context, id string) (model.Account, error) {
	var account model.Account
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&account)
	return account, utils.NotFound(err)
}

func (r accounts) List(ctx context.Context) ([]model.Account, error) {
	byName := options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).
		SetCollation(&options.Collation{Locale: "en", Strength: 2})
	cursor, err := r.collection.Find(ctx, bson.D{}, byName)
	if err != nil {
		return nil, err
	}
	list := []model.Account{}
	return list, cursor.All(ctx, &list)
}

func (r accounts) Replace(ctx context.Context, account model.Account) error {
	_, err := r.collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: account.ID}}, account)
	return err
}
