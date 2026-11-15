package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// CompaniesService serves the Tools for Companies.
type CompaniesService struct {
	companies     model.CompaniesRepository
	opportunities model.OpportunitiesRepository
	// opportunitiesService shows a Company's Opportunities in short.
	opportunitiesService *OpportunitiesService
}

// NewCompanies returns the CompaniesService.
func NewCompanies(companies model.CompaniesRepository, opportunities model.OpportunitiesRepository, opportunitiesService *OpportunitiesService) *CompaniesService {
	return &CompaniesService{companies: companies, opportunities: opportunities, opportunitiesService: opportunitiesService}
}

// Tools are what the run and the Owner's chat keep Companies with.
func (s *CompaniesService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_save_company",
		Description: "Creates a Company, or, with id, updates one: only the fields sent change. " +
			"brief is the Company Brief, a summary from public information only (what they do, size, tech, reputation, red flags), " +
			"stopping where only the Owner could add more; notes are anything else worth keeping. " +
			"A new Company needs a name no Company has. Answers the Company with its Opportunities.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string","description":"The Company to update; leave out to create one."},` +
			`"name":{"type":"string"},` +
			`"linkedInUrl":{"type":"string"},` +
			`"websiteUrl":{"type":"string"},` +
			`"brief":{"type":"string"},` +
			`"notes":{"type":"string"}}}`),
		Handle: s.saveCompanyTool,
	}, {
		Name:        "career_get_company",
		Description: "A Company, with its Company Brief, notes and Opportunities. Answers the Company.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		ReadOnly:    true,
		Handle:      s.getCompanyTool,
	}}
}

func (s *CompaniesService) saveCompanyTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID          string  `json:"id"`
		Name        *string `json:"name"`
		LinkedInURL *string `json:"linkedInUrl"`
		WebsiteURL  *string `json:"websiteUrl"`
		Brief       *string `json:"brief"`
		Notes       *string `json:"notes"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	var company model.Company
	if args.ID != "" {
		var err error
		if company, err = s.company(ctx, args.ID); err != nil {
			return nil, err
		}
	} else if args.Name == nil {
		return nil, errors.New("a new Company needs its name")
	}
	if args.Name != nil {
		name := strings.TrimSpace(*args.Name)
		if name == "" {
			return nil, errors.New("a Company's name can't be empty")
		}
		other, err := s.companies.GetByName(ctx, name)
		if err == nil && other.ID != company.ID {
			return nil, fmt.Errorf("the Company %s (%s) already has that name", other.ID.Hex(), other.Name)
		}
		if err != nil && !errors.Is(err, coreutils.ErrNotFound) {
			return nil, err
		}
		company.Name = name
	}
	for field, value := range map[*string]*string{
		&company.LinkedInURL: args.LinkedInURL, &company.WebsiteURL: args.WebsiteURL,
		&company.Brief: args.Brief, &company.Notes: args.Notes,
	} {
		if value != nil {
			*field = strings.TrimSpace(*value)
		}
	}
	var err error
	if args.ID == "" {
		company, err = s.companies.Insert(ctx, company)
	} else {
		err = s.companies.Replace(ctx, company)
	}
	if err != nil {
		return nil, err
	}
	return s.companyJSON(ctx, company)
}

func (s *CompaniesService) getCompanyTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args idArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	company, err := s.company(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	return s.companyJSON(ctx, company)
}

// companyJSON is the Company with its Opportunities.
func (s *CompaniesService) companyJSON(ctx context.Context, c model.Company) (model.CompanyJSON, error) {
	opportunities, err := s.opportunities.ByCompany(ctx, c.ID)
	if err != nil {
		return model.CompanyJSON{}, err
	}
	j := model.CompanyJSON{
		ID: c.ID.Hex(), Name: c.Name, LinkedInURL: c.LinkedInURL, WebsiteURL: c.WebsiteURL, Brief: c.Brief, Notes: c.Notes,
		Opportunities: []model.OpportunityJSON{},
	}
	if j.Opportunities, err = s.opportunitiesService.shortAll(ctx, opportunities); err != nil {
		return model.CompanyJSON{}, err
	}
	return j, nil
}

// company returns the Company with the id from a Tool's arguments.
func (s *CompaniesService) company(ctx context.Context, hex string) (model.Company, error) {
	id, err := careerutils.ID("Company", hex)
	if err != nil {
		return model.Company{}, err
	}
	company, err := s.companies.GetByID(ctx, id)
	return company, careerutils.Found(err, "Company", hex)
}
