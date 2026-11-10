package model

// CreatedJSON is what the API answers a POST that created something with: the
// new thing's ID.
type CreatedJSON struct {
	ID string `json:"id"`
}
