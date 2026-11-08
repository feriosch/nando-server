// Package model declares the Calories Module's data and one repository per
// collection that stores it. The Module's service depends on these
// interfaces, never on the repository implementations.
package model

import "errors"

// ErrNotFound means the Meal or Entry asked for doesn't exist.
var ErrNotFound = errors.New("not found")
