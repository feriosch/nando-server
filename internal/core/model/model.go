// Package model declares Core's data, the repository that stores it, and the
// service other Modules call. Services depend on it, never on implementations.
package model

// CoreRepository reads and writes Core's data. Only Core's service uses it.
// Its methods arrive with Core's first features (#7, #8).
type CoreRepository interface{}

// CoreService is what other Modules' services may ask of Core. Its methods
// arrive with Core's first features (#7, #8).
type CoreService interface{}
