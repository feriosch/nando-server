package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// SearchService serves career_search, for the Owner's chat skills.
type SearchService struct {
	recruiters    model.RecruitersRepository
	companies     model.CompaniesRepository
	opportunities model.OpportunitiesRepository
	// opportunitiesService shows the Opportunities found in short.
	opportunitiesService *OpportunitiesService
}

// NewSearch returns the SearchService.
func NewSearch(recruiters model.RecruitersRepository, companies model.CompaniesRepository, opportunities model.OpportunitiesRepository,
	opportunitiesService *OpportunitiesService) *SearchService {
	return &SearchService{recruiters: recruiters, companies: companies, opportunities: opportunities, opportunitiesService: opportunitiesService}
}

// Tools are career_search.
func (s *SearchService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_search",
		Description: "Finds Recruiters (by name or their agency or Company), Companies (by name) and Opportunities " +
			"(by role, or by their Company's name), open or closed, ignoring case. " +
			"Answers {\"recruiters\": [{\"id\", \"name\", \"profileUrl\", \"kind\", \"organisation\"}], \"companies\": [{\"id\", \"name\"}], " +
			"\"opportunities\": [{\"id\", \"company\", \"role\", \"recruiterId\", \"stage\", \"outcome\", \"interest\"}]}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		ReadOnly:    true,
		Handle:      s.searchTool,
	}}
}

func (s *SearchService) searchTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return nil, errors.New("query can't be empty")
	}
	found := model.SearchJSON{Recruiters: []model.RecruiterJSON{}, Companies: []model.CompanyRefJSON{}, Opportunities: []model.OpportunityJSON{}}
	recruiters, err := s.recruiters.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, r := range recruiters {
		found.Recruiters = append(found.Recruiters, careerutils.ToRecruiterJSON(r))
	}
	companies, err := s.companies.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	companyIDs := []bson.ObjectID{}
	for _, c := range companies {
		found.Companies = append(found.Companies, model.CompanyRefJSON{ID: c.ID.Hex(), Name: c.Name})
		companyIDs = append(companyIDs, c.ID)
	}
	opportunities, err := s.opportunities.Search(ctx, query, companyIDs)
	if err != nil {
		return nil, err
	}
	if found.Opportunities, err = s.opportunitiesService.shortAll(ctx, opportunities); err != nil {
		return nil, err
	}
	return found, nil
}
