// Package utils holds helpers the Calories Module's layers share: the checks
// on what the Owner sends, and their limits.
package utils

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Limits on what the Owner sends.
const (
	MaxNameLength = 100
	MaxKcal       = 10000
	MaxServings   = 100
	// MaxEmojiLength is in code points: enough for one emoji built of
	// several, such as a family or a flag.
	MaxEmojiLength = 8
	MaxTagLength   = 30
	MaxTags        = 20
)

// CheckName trims name and checks it is 1 to 100 characters.
func CheckName(field, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return "", errors.New(field + " must be 1 to 100 characters")
	}
	return name, nil
}

// CheckKcal checks that kcal is set and a whole number from 0 to 10,000.
func CheckKcal(field string, kcal *int) (int, error) {
	if kcal == nil || *kcal < 0 || *kcal > MaxKcal {
		return 0, errors.New(field + " must be a whole number from 0 to 10000")
	}
	return *kcal, nil
}

// CheckServings checks that servings is a whole number from 1 to 100.
func CheckServings(servings int) error {
	if servings < 1 || servings > MaxServings {
		return errors.New("servings must be a whole number from 1 to 100")
	}
	return nil
}

// CheckOptional trims s and checks it is at most max characters; nil or blank
// is "", not set.
func CheckOptional(field string, s *string, max int) (string, error) {
	v := ""
	if s != nil {
		v = strings.TrimSpace(*s)
	}
	if utf8.RuneCountInString(v) > max {
		return "", fmt.Errorf("%s must be at most %d characters, or null", field, max)
	}
	return v, nil
}

// CheckTags trims each Tag and checks it is 1 to 30 characters, dropping
// repeats; there may be at most 20.
func CheckTags(tags []string) ([]string, error) {
	checked := []string{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || utf8.RuneCountInString(tag) > MaxTagLength {
			return nil, errors.New("each tag must be 1 to 30 characters")
		}
		if !slices.Contains(checked, tag) {
			checked = append(checked, tag)
		}
	}
	if len(checked) > MaxTags {
		return nil, errors.New("a Meal can have at most 20 tags")
	}
	return checked, nil
}
