// Package matcher scores a JD extraction against a user extraction by
// counting JD items that share at least one whitespace token with any user
// item in the same category (experience / competency / trait).
//
// The scoring is intentionally simple — no embeddings, no LLM re-call.
// Synonyms and morphological variants are not handled; that is the cost of
// keeping matching cheap and deterministic.
package matcher

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Triple is the parsed shape of an extraction row (experience / competency
// / trait), already decoded from the JSON columns on either side.
type Triple struct {
	Experience []string
	Competency []string
	Trait      []string
}

// CategoryScore captures the count of matched JD items in one category and
// the JD item strings themselves (for surfacing the "왜 매칭됐는지" in the
// email body).
type CategoryScore struct {
	Hits    int
	Matched []string
}

// Score is the aggregate match result for one (user, jd) pair.
type Score struct {
	Experience CategoryScore
	Competency CategoryScore
	Trait      CategoryScore
	Total      int
}

// Match scores jd against user. Total = Experience.Hits + Competency.Hits +
// Trait.Hits. All weights equal — keep simple, iterate later if needed.
func Match(user, jd Triple) Score {
	exp := scoreCategory(user.Experience, jd.Experience)
	comp := scoreCategory(user.Competency, jd.Competency)
	trait := scoreCategory(user.Trait, jd.Trait)
	return Score{
		Experience: exp,
		Competency: comp,
		Trait:      trait,
		Total:      exp.Hits + comp.Hits + trait.Hits,
	}
}

// scoreCategory returns how many jd items share ≥1 token with the union of
// tokens drawn from user items. The matched jd strings are returned for
// rendering.
func scoreCategory(userItems, jdItems []string) CategoryScore {
	if len(userItems) == 0 || len(jdItems) == 0 {
		return CategoryScore{}
	}

	userTokens := make(map[string]struct{})
	for _, u := range userItems {
		for tok := range tokenize(u) {
			userTokens[tok] = struct{}{}
		}
	}
	if len(userTokens) == 0 {
		return CategoryScore{}
	}

	out := CategoryScore{}
	for _, jd := range jdItems {
		jdTokens := tokenize(jd)
		for tok := range jdTokens {
			if _, ok := userTokens[tok]; ok {
				out.Hits++
				out.Matched = append(out.Matched, jd)
				break
			}
		}
	}
	return out
}

// tokenize lowercases the string, replaces non-letter/non-digit runes with
// whitespace (except `#` and `+`, which are kept so language names like
// "C#" / "C++" survive as tokens), and returns the set of tokens with
// rune length ≥ 2 (filters out single-character noise like "을", "의").
func tokenize(s string) map[string]struct{} {
	s = strings.ToLower(s)

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '#' || r == '+' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}

	out := make(map[string]struct{})
	for _, tok := range strings.Fields(b.String()) {
		if utf8.RuneCountInString(tok) < 2 {
			continue
		}
		out[tok] = struct{}{}
	}
	return out
}
