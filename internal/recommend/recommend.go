// Package recommend holds the rule-based job-matching rule: which candidate
// postings to surface for a user from plain crawler-parsed fields and the
// user's stored preferences, with no LLM extraction involved.
//
// It is the single source of truth for the "회사·경력 매칭" rule shared by
// cmd/recommend (rule-based only) and cmd/notify (rule-based "필수 표시"
// section that sits alongside the LLM-scored recommendations).
package recommend

import (
	"skipjd/internal/gamejob"
	"skipjd/internal/model"
)

// careerSlack is how many years above the user's experience a posting may
// still require and remain eligible. Lenient by design: unknown experience on
// either side never excludes a posting.
const careerSlack = 3

// Rule is the compiled form of the rule-based criteria: the user's company
// preferences normalized into a set plus their experience years. Build one
// with NewRule, then test individual postings with Matches.
type Rule struct {
	// companies is nil when the user has no company preference, which means
	// "no company filter" — every company passes.
	companies   map[string]struct{}
	careerYears *int
}

// NewRule compiles companyNames and careerYears into a Rule. An empty
// companyNames means "no company filter".
func NewRule(companyNames []string, careerYears *int) Rule {
	var companies map[string]struct{}
	if len(companyNames) > 0 {
		companies = make(map[string]struct{}, len(companyNames))
		for _, name := range companyNames {
			companies[gamejob.NormalizeCompanyName(name)] = struct{}{}
		}
	}
	return Rule{companies: companies, careerYears: careerYears}
}

// Matches reports whether a single posting satisfies the rule-based criteria:
//
//   - the posting's company matches one of the rule's companies
//     (NormalizeCompanyName applied to both sides), unless the rule has no
//     company filter;
//   - the posting's MinExperienceYears is nil, or the rule's careerYears is
//     nil, or MinExperienceYears <= careerYears + careerSlack.
//
// Duty-code overlap is assumed already enforced by candidate selection (see
// repository.ListJobPostingsByDutyCodes).
func (r Rule) Matches(p model.JobPosting) bool {
	if r.companies != nil {
		if _, ok := r.companies[gamejob.NormalizeCompanyName(p.Company)]; !ok {
			return false
		}
	}
	if p.MinExperienceYears != nil && r.careerYears != nil &&
		*p.MinExperienceYears > *r.careerYears+careerSlack {
		return false
	}
	return true
}

// Match returns the subset of candidates that satisfy
// NewRule(companyNames, careerYears). Input order is preserved.
func Match(candidates []model.JobPosting, companyNames []string, careerYears *int) []model.JobPosting {
	rule := NewRule(companyNames, careerYears)
	out := make([]model.JobPosting, 0, len(candidates))
	for i := range candidates {
		if rule.Matches(candidates[i]) {
			out = append(out, candidates[i])
		}
	}
	return out
}
