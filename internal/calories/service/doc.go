// Package service handles the Calories Module's requests and validates them:
// MealsService for Meals and EntriesService for Entries. It never touches the
// database; it reaches its own data only through the MealsRepository and
// EntriesRepository, and asks Core for Core's data through CoreService.
package service
