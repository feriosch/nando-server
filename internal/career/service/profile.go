package service

import (
	"context"
	"encoding/json"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// ProfileService serves the Career Profile's Tools, the only way to read or
// write it: the app never shows it.
type ProfileService struct {
	profile model.ProfileRepository
}

// NewProfile returns the ProfileService.
func NewProfile(profile model.ProfileRepository) *ProfileService {
	return &ProfileService{profile: profile}
}

// Tools read and write the Career Profile.
func (s *ProfileService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_get_profile",
		Description: "The Career Profile: what Claude knows about the Owner's working life, to judge Opportunities and write Suggested Replies, " +
			"such as their resumes, current situation, what they are looking for and their deal-breakers. " +
			"One Markdown document, empty until it is first saved. Answers {\"markdown\"}.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		ReadOnly:    true,
		Handle:      s.getTool,
	}, {
		Name: "career_save_profile",
		Description: "Replaces the whole Career Profile with markdown: read it with career_get_profile first and send it back changed. " +
			"Keep only what was taken from files the Owner shares, never the files. Answers {\"markdown\"}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"markdown":{"type":"string"}},"required":["markdown"]}`),
		Handle:      s.saveTool,
	}}
}

func (s *ProfileService) getTool(ctx context.Context, _ json.RawMessage) (any, error) {
	markdown, err := s.profile.Find(ctx)
	return model.ProfileJSON{Markdown: markdown}, err
}

func (s *ProfileService) saveTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.ProfileJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	return args, s.profile.Save(ctx, args.Markdown)
}
