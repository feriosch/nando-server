// Package model declares the Calories Module's data and one repository per
// collection that stores it. The Module's service depends on these
// interfaces, never on the repository implementations.
package model

import coremodel "github.com/feriosch/nando-server/internal/core/model"

// ErrNotFound means the Meal or Entry asked for doesn't exist. It is Core's,
// so Core's repository helpers report it too.
var ErrNotFound = coremodel.ErrNotFound
