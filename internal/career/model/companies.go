package model

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Company is an employer that has been put to the Owner in at least one
// Opportunity.
type Company struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	Name        string        `bson:"name"`
	LinkedInURL string        `bson:"linkedInUrl"`
	WebsiteURL  string        `bson:"websiteUrl"`
	// Brief is the Company Brief: Claude's summary from public information.
	Brief string `bson:"brief"`
	Notes string `bson:"notes"`
}

// CompaniesRepository reads and writes the companies collection.
type CompaniesRepository interface {
	// Insert stores a new Company and returns it with its id.
	Insert(ctx context.Context, company Company) (Company, error)
	// GetByID returns the Company, or Core's utils.ErrNotFound.
	GetByID(ctx context.Context, id bson.ObjectID) (Company, error)
	// GetByName returns the Company named name, ignoring case, or Core's
	// utils.ErrNotFound.
	GetByName(ctx context.Context, name string) (Company, error)
	// Replace stores the Company over the one with its id.
	Replace(ctx context.Context, company Company) error
	// Search returns the Companies whose name has query in it, ignoring
	// case.
	Search(ctx context.Context, query string) ([]Company, error)
}

// The JSON shapes below are what the Tools answer with and are sent.

// CompanyRefJSON names a Company: an existing one by id, or, when sent, a
// new one by name.
type CompanyRefJSON struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// CompanyJSON is a Company as the Tools answer it, with its Opportunities.
type CompanyJSON struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	LinkedInURL   string            `json:"linkedInUrl"`
	WebsiteURL    string            `json:"websiteUrl"`
	Brief         string            `json:"brief"`
	Notes         string            `json:"notes"`
	Opportunities []OpportunityJSON `json:"opportunities"`
}
