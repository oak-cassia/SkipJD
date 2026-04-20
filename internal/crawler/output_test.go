package crawler

import (
	"testing"
	"time"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeProducesCrawlerJSONShape(t *testing.T) {
	minYears := 3

	outputText, err := Encode([]model.JobPosting{{
		Source:             gamejob.SourceName,
		SourceKey:          "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
		Title:              "Server Engineer",
		Company:            "에피드게임즈",
		ClosingDate:        "채용시",
		URL:                "https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868",
		MinExperienceYears: &minYears,
	}}, map[string][]int{
		"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868": {1, 3},
	})
	require.NoError(t, err)

	assert.JSONEq(t, `{"postings":[{"title":"Server Engineer","company":"에피드게임즈","duty_codes":[1,3],"url":"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=275868","closing_date":"채용시","min_experience_years":3}]}`, outputText)
}

func TestParseBuildsJobPostingModels(t *testing.T) {
	seenAt := time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC)
	outputText := `{"postings":[{"title":"Backend Engineer","company":"Krafton","duty_codes":[16],"url":"https://jobs.example.com/postings/123#details","closing_date":"채용 시 마감","min_experience_years":3},{"title":"AI Engineer","company":"Krafton","url":"https://jobs.example.com/postings/456","closing_date":"2026-04-01"}]}`

	postings, dutyCodesBySourceKey, err := Parse(outputText, seenAt)
	require.NoError(t, err)
	require.Len(t, postings, 2)

	assert.Equal(t, gamejob.SourceName, postings[0].Source)
	assert.Equal(t, "https://jobs.example.com/postings/123", postings[0].SourceKey)
	assert.Equal(t, "Backend Engineer", postings[0].Title)
	assert.Equal(t, "Krafton", postings[0].Company)
	assert.Equal(t, "https://jobs.example.com/postings/123#details", postings[0].URL)
	assert.Equal(t, "채용 시 마감", postings[0].ClosingDate)
	require.NotNil(t, postings[0].MinExperienceYears)
	assert.Equal(t, 3, *postings[0].MinExperienceYears)
	assert.True(t, postings[0].FirstSeenAt.Equal(seenAt))
	assert.True(t, postings[0].LastSeenAt.Equal(seenAt))

	assert.Equal(t, "https://jobs.example.com/postings/456", postings[1].SourceKey)
	assert.Nil(t, postings[1].MinExperienceYears)
	assert.Equal(t, map[string][]int{
		"https://jobs.example.com/postings/123": {16},
	}, dutyCodesBySourceKey)
}

func TestParseRejectsMissingRequiredFields(t *testing.T) {
	_, _, err := Parse(`{"postings":[{"company":"Krafton","url":"https://jobs.example.com/postings/123","closing_date":"채용 시 마감"}]}`, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestParseAcceptsGameJobDetailURL(t *testing.T) {
	_, _, err := Parse(`{"postings":[{"title":"[NineB/Project PX] Server Programmer (5년 이상)","company":"Krafton","url":"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=274647","closing_date":"채용시","min_experience_years":5}]}`, time.Now())
	require.NoError(t, err)
}

func TestParseRejectsGameJobListingURL(t *testing.T) {
	_, _, err := Parse(`{"postings":[{"title":"[Palworld Mobile] Engine Eng...","company":"Krafton","url":"https://www.gamejob.co.kr/Recruit/joblist?menucode=duty&duty=1","closing_date":"채용시","min_experience_years":0}]}`, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GameJob detail page")
}

func TestParseRejectsTruncatedGameJobTitle(t *testing.T) {
	_, _, err := Parse(`{"postings":[{"title":"[Palworld Mobile] Engine Eng...","company":"Krafton","url":"https://www.gamejob.co.kr/Recruit/GI_Read/View?GI_No=277887","closing_date":"채용시","min_experience_years":0}]}`, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title appears truncated")
}
