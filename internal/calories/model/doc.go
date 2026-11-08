// Package model declares the Calories Module's data and one repository per
// collection that stores it. The Module's service depends on these
// interfaces, never on the repository implementations. A repository reports a
// missing Meal or Entry with Core's model.ErrNotFound.
package model
