package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/japanese/repository"
	"github.com/feriosch/nando-server/internal/testmongo"
)

// The kanji collection came from benkyo's dump, indexes included, so it may
// already have an index on kanji under another name or with other options.
func TestTheKanjiCollectionStartsWithAnyExistingIndexOnKanji(t *testing.T) {
	client, err := mongo.Connect(options.Client().ApplyURI(testmongo.URI(t)))
	if err != nil {
		t.Fatalf("connect MongoDB: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	byKanji := bson.D{{Key: "kanji", Value: 1}}

	for name, existing := range map[string]mongo.IndexModel{
		"default name":     {Keys: byKanji},
		"another name":     {Keys: byKanji, Options: options.Index().SetName("by_character")},
		"unique":           {Keys: byKanji, Options: options.Index().SetUnique(true)},
		"none (a new one)": {},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := client.Database("japanese_" + bson.NewObjectID().Hex())
			if existing.Keys != nil {
				if _, err := db.Collection("kanji").Indexes().CreateOne(ctx, existing); err != nil {
					t.Fatalf("create the existing index: %v", err)
				}
			}
			if _, err := repository.NewKanji(ctx, db); err != nil {
				t.Errorf("NewKanji: %v", err)
			}
		})
	}
}
