package model

import "context"

// FileStore is where every Statement's file is kept for good (ADR 0014): a
// private Cloud Storage bucket, which Nando may add to and read, never
// overwrite or delete from.
type FileStore interface {
	// Put keeps data as the object name, which must be new.
	Put(ctx context.Context, name string, data []byte, contentType string) error
	// Get reads the object name back.
	Get(ctx context.Context, name string) ([]byte, error)
}
