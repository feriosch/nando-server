package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLength = 100
	maxKcal       = 10000
	maxServings   = 100
	// maxEmojiLength is in code points: enough for one emoji built of
	// several, such as a family or a flag.
	maxEmojiLength = 8
	maxTagLength   = 30
	maxTags        = 20
)

// checkName trims name and checks it is 1 to 100 characters.
func checkName(field, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return "", errors.New(field + " must be 1 to 100 characters")
	}
	return name, nil
}

// checkKcal checks that kcal is set and a whole number from 0 to 10,000.
func checkKcal(field string, kcal *int) (int, error) {
	if kcal == nil || *kcal < 0 || *kcal > maxKcal {
		return 0, errors.New(field + " must be a whole number from 0 to 10000")
	}
	return *kcal, nil
}

// checkServings checks that servings is a whole number from 1 to 100.
func checkServings(servings int) error {
	if servings < 1 || servings > maxServings {
		return errors.New("servings must be a whole number from 1 to 100")
	}
	return nil
}

// checkOptional trims s and checks it is at most max characters; nil or blank
// is "", not set.
func checkOptional(field string, s *string, max int) (string, error) {
	v := strings.TrimSpace(orEmpty(s))
	if utf8.RuneCountInString(v) > max {
		return "", fmt.Errorf("%s must be at most %d characters, or null", field, max)
	}
	return v, nil
}

// checkTags trims each Tag and checks it is 1 to 30 characters, dropping
// repeats; there may be at most 20.
func checkTags(tags []string) ([]string, error) {
	checked := []string{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || utf8.RuneCountInString(tag) > maxTagLength {
			return nil, errors.New("each tag must be 1 to 30 characters")
		}
		if !slices.Contains(checked, tag) {
			checked = append(checked, tag)
		}
	}
	if len(checked) > maxTags {
		return nil, errors.New("a Meal can have at most 20 tags")
	}
	return checked, nil
}
