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
	conversations model.ConversationsRepository
	companies     model.CompaniesRepository
	opportunities model.OpportunitiesRepository
	// opportunitiesService shows the Opportunities found in short.
	opportunitiesService *OpportunitiesService
}

// NewSearch returns the SearchService.
func NewSearch(recruiters model.RecruitersRepository, conversations model.ConversationsRepository, companies model.CompaniesRepository,
	opportunities model.OpportunitiesRepository, opportunitiesService *OpportunitiesService) *SearchService {
	return &SearchService{recruiters: recruiters, conversations: conversations, companies: companies, opportunities: opportunities,
		opportunitiesService: opportunitiesService}
}

// Tools are career_search.
func (s *SearchService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_search",
		Description: "Finds Recruiters (by name or their agency or Company), Companies (by name) and Opportunities " +
			"(by role, or by their Company's name), open or closed, ignoring case. A Recruiter comes with their Conversations, " +
			"each with whether it has a Suggested Reply (career_get_conversation reads one), and the Opportunities they brought, " +
			"in short (none for a Lead; career_get_opportunity reads one). " +
			"Answers {\"recruiters\": [{\"id\", \"name\", \"profileUrl\", \"kind\", \"organisation\", " +
			"\"conversations\": [{\"id\", \"hasSuggestedReply\"}], \"opportunities\": [{\"id\", \"company\", \"role\", \"stage\", \"outcome\", \"interest\"}]}], \"companies\": [{\"id\", \"name\"}], " +
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
	found := model.SearchJSON{Recruiters: []model.FoundRecruiterJSON{}, Companies: []model.CompanyRefJSON{}, Opportunities: []model.OpportunityJSON{}}
	recruiters, err := s.recruiters.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, r := range recruiters {
		filed, err := s.conversations.ByRecruiter(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		j := model.FoundRecruiterJSON{RecruiterJSON: careerutils.ToRecruiterJSON(r), Conversations: []model.FoundConversationJSON{}}
		for _, c := range filed {
			j.Conversations = append(j.Conversations, model.FoundConversationJSON{ID: c.ID.Hex(), HasSuggestedReply: c.SuggestedReply != nil})
		}
		if j.Opportunities, err = s.opportunitiesService.ByRecruiter(ctx, r.ID); err != nil {
			return nil, err
		}
		found.Recruiters = append(found.Recruiters, j)
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
