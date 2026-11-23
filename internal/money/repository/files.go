package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"google.golang.org/api/googleapi"
	storage "google.golang.org/api/storage/v1"

	"github.com/feriosch/nando-server/internal/money/model"
)

// NewBucket returns the FileStore over the Cloud Storage bucket named
// bucket, signed in as Nando's own service account (Cloud Run's default
// credentials).
func NewBucket(ctx context.Context, bucket string) (model.FileStore, error) {
	service, err := storage.NewService(ctx)
	if err != nil {
		return nil, err
	}
	return gcsBucket{objects: service.Objects, bucket: bucket}, nil
}

// gcsBucket is the Statements bucket in Cloud Storage.
type gcsBucket struct {
	objects *storage.ObjectsService
	bucket  string
}

func (b gcsBucket) Put(ctx context.Context, name string, data []byte, contentType string) error {
	// ifGenerationMatch=0: only create, never overwrite, whatever the
	// account may do.
	_, err := b.objects.Insert(b.bucket, &storage.Object{Name: name, ContentType: contentType}).
		IfGenerationMatch(0).Media(bytes.NewReader(data), googleapi.ContentType(contentType)).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("keeping %s in the bucket: %w", name, err)
	}
	return nil
}

func (b gcsBucket) Get(ctx context.Context, name string) ([]byte, error) {
	resp, err := b.objects.Get(b.bucket, name).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("reading %s from the bucket: %w", name, err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
