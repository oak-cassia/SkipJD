package recommend

import (
	"testing"

	"skipjd/internal/model"
)

func intPtr(v int) *int { return &v }

func ids(postings []model.JobPosting) []uint {
	out := make([]uint, len(postings))
	for i := range postings {
		out[i] = postings[i].ID
	}
	return out
}

func TestRuleMatches(t *testing.T) {
	tests := []struct {
		name        string
		companies   []string
		careerYears *int
		posting     model.JobPosting
		want        bool
	}{
		{
			name:      "company normalized on both sides",
			companies: []string{"㈜넵튠"},
			posting:   model.JobPosting{Company: "넵튠"},
			want:      true,
		},
		{
			name:      "company not preferred",
			companies: []string{"크래프톤"},
			posting:   model.JobPosting{Company: "넥슨"},
			want:      false,
		},
		{
			name:        "no company filter passes any company",
			companies:   nil,
			careerYears: intPtr(1),
			posting:     model.JobPosting{Company: "기타회사", MinExperienceYears: intPtr(4)},
			want:        true,
		},
		{
			name:        "career at slack boundary passes",
			companies:   []string{"크래프톤"},
			careerYears: intPtr(2), // slack +3 → max 5
			posting:     model.JobPosting{Company: "크래프톤", MinExperienceYears: intPtr(5)},
			want:        true,
		},
		{
			name:        "career over slack boundary fails",
			companies:   []string{"크래프톤"},
			careerYears: intPtr(2), // slack +3 → max 5
			posting:     model.JobPosting{Company: "크래프톤", MinExperienceYears: intPtr(6)},
			want:        false,
		},
		{
			name:        "nil min experience never excludes",
			companies:   []string{"크래프톤"},
			careerYears: intPtr(0),
			posting:     model.JobPosting{Company: "크래프톤", MinExperienceYears: nil},
			want:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rule := NewRule(tc.companies, tc.careerYears)
			if got := rule.Matches(tc.posting); got != tc.want {
				t.Fatalf("Matches() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	candidates := []model.JobPosting{
		{ID: 1, Company: "크래프톤", MinExperienceYears: intPtr(3)},
		{ID: 2, Company: "㈜넵튠", MinExperienceYears: intPtr(10)}, // company normalized
		{ID: 3, Company: "넥슨", MinExperienceYears: nil},         // unknown experience never excludes
		{ID: 4, Company: "기타회사", MinExperienceYears: intPtr(2)}, // company not preferred
	}

	tests := []struct {
		name        string
		companies   []string
		careerYears *int
		want        []uint
	}{
		{
			name:        "company filter keeps only preferred",
			companies:   []string{"크래프톤", "넵튠", "넥슨"},
			careerYears: nil,
			want:        []uint{1, 2, 3},
		},
		{
			name:        "career slack excludes far-over postings",
			companies:   []string{"크래프톤", "넵튠", "넥슨"},
			careerYears: intPtr(3), // slack +3 → max 6; id=2 needs 10 → excluded
			want:        []uint{1, 3},
		},
		{
			name:        "empty company list means no company filter",
			companies:   nil,
			careerYears: intPtr(0), // slack +3 → max 3; id=2 needs 10 excluded, id=4 needs 2 ok
			want:        []uint{1, 3, 4},
		},
		{
			name:        "nil career years never excludes on experience",
			companies:   []string{"넵튠"},
			careerYears: nil,
			want:        []uint{2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ids(Match(candidates, tc.companies, tc.careerYears))
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}
