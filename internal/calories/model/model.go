// Package model declares the Calories Module's data and the repository that
// stores it. The Module's service depends on it, never on the repository
// implementation.
package model

// CaloriesRepository reads and writes the Calories Module's data. Its methods
// arrive with the first Calories feature (#10).
type CaloriesRepository interface{}
