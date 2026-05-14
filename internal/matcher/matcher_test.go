package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchKoreanCompoundWords(t *testing.T) {
	user := Triple{
		Experience: []string{"Unity 5년 개발", "라이브 서비스 운영 경험"},
		Competency: []string{"C# 숙련", "MySQL 경험"},
		Trait:      []string{"주도적인 문제 해결"},
	}
	jd := Triple{
		Experience: []string{"Unity 3년 이상", "모바일 라이브 서비스 운영"},
		Competency: []string{"C# 능숙", "Git 협업"},
		Trait:      []string{"원활한 커뮤니케이션"},
	}

	score := Match(user, jd)

	assert.Equal(t, 2, score.Experience.Hits, "Unity, 라이브/서비스/운영 두 항목 모두 매칭")
	assert.Equal(t, 1, score.Competency.Hits, "C# 한 항목 매칭")
	assert.Equal(t, 0, score.Trait.Hits, "trait 토큰 겹침 없음")
	assert.Equal(t, 3, score.Total)
}

func TestMatchEmptySides(t *testing.T) {
	score := Match(Triple{}, Triple{Experience: []string{"foo"}})
	assert.Equal(t, 0, score.Total)
	assert.Empty(t, score.Experience.Matched)
}

func TestTokenizeDropsShortAndPunctuation(t *testing.T) {
	got := tokenize("C# 숙련, PHP/MySQL")
	assert.Contains(t, got, "숙련")
	assert.Contains(t, got, "php")
	assert.Contains(t, got, "mysql")
	assert.NotContains(t, got, "c", "1-char token must be dropped")
}
