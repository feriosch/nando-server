package model

// The JSON shapes below are what the Sync Tools answer with and are sent.

// SyncStatusJSON is what the Mac's Sync script is told before it reads
// LinkedIn: whether a Sync is due now.
type SyncStatusJSON struct {
	Due bool `json:"due"`
}
