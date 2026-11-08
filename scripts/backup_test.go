// Tests for the weekly backup (.github/workflows/backup.yml): backup.sh is run
// the way the workflow runs it, inside the mongo:8 image, and its archive is
// restored with the README's mongorestore command into a fresh MongoDB.
package scripts_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// insideURI is how a process inside a MongoDB container reaches its mongod.
const insideURI = "mongodb://localhost:27017"

// mongoServer is a fresh mongo:8 container (which also ships mongodump and
// mongorestore) and a client connected to it from the test.
type mongoServer struct {
	container *mongodb.MongoDBContainer
	client    *mongo.Client
}

func startMongo(t *testing.T, opts ...testcontainers.ContainerCustomizer) *mongoServer {
	t.Helper()
	ctx := context.Background()
	container, err := mongodb.Run(ctx, "mongo:8", opts...)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start MongoDB container (is Docker running?): %v", err)
	}
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("MongoDB container connection string: %v", err)
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return &mongoServer{container: container, client: client}
}

// run runs cmd inside the container with env and fails the test unless it
// exits 0.
func (m *mongoServer) run(t *testing.T, env []string, cmd ...string) {
	t.Helper()
	code, out, err := m.container.Exec(context.Background(), cmd, tcexec.Multiplexed(), tcexec.WithEnv(env))
	if err != nil {
		t.Fatalf("exec %v: %v", cmd, err)
	}
	output, _ := io.ReadAll(out)
	if code != 0 {
		t.Fatalf("%v exited %d:\n%s", cmd, code, output)
	}
}

func (m *mongoServer) insert(t *testing.T, db, collection string, doc bson.D) {
	t.Helper()
	if _, err := m.client.Database(db).Collection(collection).InsertOne(context.Background(), doc); err != nil {
		t.Fatalf("insert into %s.%s: %v", db, collection, err)
	}
}

// find returns the documents of db.collection, sorted by _id.
func (m *mongoServer) find(t *testing.T, db, collection string) []bson.M {
	t.Helper()
	ctx := context.Background()
	cur, err := m.client.Database(db).Collection(collection).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		t.Fatalf("find in %s.%s: %v", db, collection, err)
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		t.Fatalf("read %s.%s: %v", db, collection, err)
	}
	return docs
}

// backup runs scripts/backup.sh against m, as the workflow does, and returns
// the path of the archive it wrote, copied out to the test's temp dir.
func backup(t *testing.T, m *mongoServer) string {
	t.Helper()
	return backupAs(t, m, insideURI)
}

// backupAs is backup connecting with uri, from inside m's container.
func backupAs(t *testing.T, m *mongoServer, uri string) string {
	t.Helper()
	ctx := context.Background()
	if err := m.container.CopyFileToContainer(ctx, "backup.sh", "/work/scripts/backup.sh", 0o755); err != nil {
		t.Fatalf("copy backup.sh: %v", err)
	}
	m.run(t, []string{"MONGO_URI=" + uri}, "sh", "-c", "cd /work && scripts/backup.sh nando.tar")

	r, err := m.container.CopyFileFromContainer(ctx, "/work/nando.tar")
	if err != nil {
		t.Fatalf("copy archive out: %v", err)
	}
	defer r.Close()
	archive, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	path := filepath.Join(t.TempDir(), "nando.tar")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

// restore restores archive into m with the README's restore commands.
func restore(t *testing.T, m *mongoServer, archive string) {
	t.Helper()
	if err := m.container.CopyFileToContainer(context.Background(), archive, "/tmp/nando.tar", 0o644); err != nil {
		t.Fatalf("copy archive in: %v", err)
	}
	m.run(t, []string{"MONGO_URI=" + insideURI}, "sh", "-c", `cd /tmp &&
		mkdir nando && tar -xf nando.tar -C nando &&
		mongorestore --uri "$MONGO_URI" --gzip --drop nando`)
}

func TestBackupRestoresCoreAndCalories(t *testing.T) {
	source := startMongo(t)
	source.insert(t, "core", "settings", bson.D{{Key: "_id", Value: "owner"}, {Key: "timeZone", Value: "Asia/Tokyo"}})
	source.insert(t, "calories", "meals", bson.D{{Key: "_id", Value: "onigiri"}, {Key: "name", Value: "Onigiri"}, {Key: "kcal", Value: int32(180)}})

	archive := backup(t, source)
	target := startMongo(t)
	restore(t, target, archive)

	assertDocs(t, target.find(t, "core", "settings"), []bson.M{{"_id": "owner", "timeZone": "Asia/Tokyo"}})
	assertDocs(t, target.find(t, "calories", "meals"), []bson.M{{"_id": "onigiri", "name": "Onigiri", "kcal": int32(180)}})
}

func TestBackupLeavesOutOtherDatabases(t *testing.T) {
	source := startMongo(t)
	source.insert(t, "core", "settings", bson.D{{Key: "_id", Value: "owner"}})
	source.insert(t, "benkyo", "decks", bson.D{{Key: "_id", Value: "kanji"}})

	archive := backup(t, source)
	target := startMongo(t)
	restore(t, target, archive)

	names, err := target.client.ListDatabaseNames(context.Background(), bson.D{})
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	for _, name := range names {
		if name != "core" && name != "calories" && name != "admin" && name != "config" && name != "local" {
			t.Errorf("restored database %q, want only core and calories", name)
		}
	}
}

// Production's Atlas user may only readWrite core and calories (#5).
func TestBackupNeedsOnlyNandosDatabaseUser(t *testing.T) {
	source := startMongo(t, mongodb.WithUsername("root"), mongodb.WithPassword("root"))
	source.insert(t, "core", "settings", bson.D{{Key: "_id", Value: "owner"}})
	source.insert(t, "calories", "meals", bson.D{{Key: "_id", Value: "onigiri"}})
	err := source.client.Database("admin").RunCommand(context.Background(), bson.D{
		{Key: "createUser", Value: "nando"},
		{Key: "pwd", Value: "nando"},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: "core"}},
			bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: "calories"}},
		}},
	}).Err()
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	archive := backupAs(t, source, "mongodb://nando:nando@localhost:27017/?authSource=admin")
	target := startMongo(t)
	restore(t, target, archive)

	assertDocs(t, target.find(t, "core", "settings"), []bson.M{{"_id": "owner"}})
	assertDocs(t, target.find(t, "calories", "meals"), []bson.M{{"_id": "onigiri"}})
}

func assertDocs(t *testing.T, got, want []bson.M) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("documents = %v, want %v", got, want)
	}
}
