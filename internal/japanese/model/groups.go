package model

import "context"

// Group is the one place a Word is filed.
type Group struct {
	// Key is what a Word's Group holds, such as "jlpt_n2".
	Key string
	// Name is how the Group is shown, such as "JLPT N2".
	Name string
	// ImageURL is the Group's picture.
	ImageURL string
}

// GroupsRepository reads the groups collection.
type GroupsRepository interface {
	// GetOneByKey returns the Group with this key, or Core's utils.ErrNotFound.
	GetOneByKey(ctx context.Context, key string) (Group, error)
}

// The JSON shapes below are what the API answers with.

// GroupJSON is a Group as the Word page shows it.
type GroupJSON struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	ImageURL *string `json:"imageUrl"`
}
