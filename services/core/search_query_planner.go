package core

import (
	"strings"

	"jarvis-pa/packages/errors"
)

// SearchIntent classifies what the user is trying to accomplish with a search.
type SearchIntent string

const (
	IntentComparison  SearchIntent = "comparison"
	IntentFactual     SearchIntent = "factual"
	IntentHowTo       SearchIntent = "how_to"
	IntentExploratory SearchIntent = "exploratory"
	IntentNews        SearchIntent = "news"
	IntentReview      SearchIntent = "review"
)

// SearchPlan is the output of QueryPlanner.Plan: the original request broken
// into a detected intent, a set of focused sub-queries, and optional
// refinement hints for later passes.
type SearchPlan struct {
	OriginalRequest string
	Intent          SearchIntent
	Queries         []SearchQuery
	RefinementHints []string
}

// QueryPlanner converts natural-language user requests into multi-query
// search strategies.
type QueryPlanner struct{}

// NewQueryPlanner creates a QueryPlanner.
func NewQueryPlanner() *QueryPlanner {
	return &QueryPlanner{}
}

// Plan decomposes request into a SearchPlan: it detects intent, generates
// focused sub-queries, and provides refinement hints. Returns an error if
// request is empty.
func (p *QueryPlanner) Plan(request string) (SearchPlan, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		return SearchPlan{}, errors.New(errors.TypeInvalidInput, "SEARCH_PLAN_EMPTY_REQUEST",
			"core.queryplanner", "search request is empty")
	}

	intent := detectIntent(request)
	queries := decompose(request, intent)
	hints := refinementHints(intent)

	return SearchPlan{
		OriginalRequest: request,
		Intent:          intent,
		Queries:         queries,
		RefinementHints: hints,
	}, nil
}

// Refine takes an existing plan and the responses from its queries, then
// produces an improved plan: it drops queries that returned no results,
// narrows broad queries that returned too many low-relevance results, and
// adds follow-up queries suggested by gaps in coverage.
func (p *QueryPlanner) Refine(plan SearchPlan, results []SearchResponse) SearchPlan {
	refined := SearchPlan{
		OriginalRequest: plan.OriginalRequest,
		Intent:          plan.Intent,
	}

	var coveredTerms []string

	for i, resp := range results {
		if i >= len(plan.Queries) {
			break
		}
		q := plan.Queries[i]

		if len(resp.Results) == 0 {
			broader := broaden(q)
			refined.Queries = append(refined.Queries, broader)
			refined.RefinementHints = append(refined.RefinementHints,
				"broadened query: "+q.Query+" -> "+broader.Query)
			continue
		}

		if hasLowRelevance(resp) {
			narrowed := narrow(q, plan.OriginalRequest)
			refined.Queries = append(refined.Queries, narrowed)
			refined.RefinementHints = append(refined.RefinementHints,
				"narrowed query: "+q.Query+" -> "+narrowed.Query)
		} else {
			refined.Queries = append(refined.Queries, q)
		}

		coveredTerms = append(coveredTerms, extractCoveredTerms(resp)...)
	}

	gaps := findGaps(plan.OriginalRequest, coveredTerms)
	for _, gap := range gaps {
		refined.Queries = append(refined.Queries, SearchQuery{Query: gap})
		refined.RefinementHints = append(refined.RefinementHints, "gap-fill query: "+gap)
	}

	if len(refined.Queries) == 0 {
		refined.Queries = plan.Queries
	}

	return refined
}

// --- intent detection ---

var intentSignals = map[SearchIntent][]string{
	IntentComparison: {
		"compare", "vs", "versus", "difference between", "better",
		"best", "alternative", "or", "which",
	},
	IntentHowTo: {
		"how to", "how do", "tutorial", "guide", "steps to",
		"setup", "set up", "install", "configure", "build",
	},
	IntentNews: {
		"latest", "news", "recent", "update", "announce",
		"released", "launch", "today", "2024", "2025", "2026",
	},
	IntentReview: {
		"review", "opinion", "experience", "feedback",
		"pros and cons", "worth", "recommend",
	},
	IntentFactual: {
		"what is", "what are", "define", "meaning", "who is",
		"when did", "where is", "how many", "how much",
	},
}

func detectIntent(request string) SearchIntent {
	lower := strings.ToLower(request)

	best := IntentExploratory
	bestScore := 0

	for intent, signals := range intentSignals {
		score := 0
		for _, signal := range signals {
			if strings.Contains(lower, signal) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = intent
		}
	}

	return best
}

// --- query decomposition ---

func decompose(request string, intent SearchIntent) []SearchQuery {
	switch intent {
	case IntentComparison:
		return decomposeComparison(request)
	case IntentHowTo:
		return decomposeHowTo(request)
	case IntentNews:
		return decomposeNews(request)
	case IntentReview:
		return decomposeReview(request)
	case IntentFactual:
		return decomposeFactual(request)
	default:
		return decomposeExploratory(request)
	}
}

func decomposeComparison(request string) []SearchQuery {
	queries := []SearchQuery{
		{Query: request},
	}

	lower := strings.ToLower(request)
	subjects := extractComparisonSubjects(lower)

	if len(subjects) >= 2 {
		for _, subj := range subjects {
			queries = append(queries, SearchQuery{
				Query: subj + " features specifications",
			})
		}
		queries = append(queries, SearchQuery{
			Query: strings.Join(subjects, " vs ") + " benchmark comparison",
		})
	} else {
		words := significantWords(request)
		topic := strings.Join(words, " ")
		queries = append(queries, SearchQuery{
			Query: topic + " comparison benchmark",
		})
		queries = append(queries, SearchQuery{
			Query: topic + " reviews recommendations",
		})
	}

	return queries
}

func decomposeHowTo(request string) []SearchQuery {
	return []SearchQuery{
		{Query: request},
		{Query: request + " step by step"},
		{Query: request + " common issues troubleshooting"},
	}
}

func decomposeNews(request string) []SearchQuery {
	return []SearchQuery{
		{Query: request, TimeRange: "month"},
		{Query: request, TimeRange: "week"},
	}
}

func decomposeReview(request string) []SearchQuery {
	return []SearchQuery{
		{Query: request},
		{Query: request + " user experience"},
		{Query: request + " pros cons"},
	}
}

func decomposeFactual(request string) []SearchQuery {
	return []SearchQuery{
		{Query: request},
	}
}

func decomposeExploratory(request string) []SearchQuery {
	words := significantWords(request)

	queries := []SearchQuery{
		{Query: request},
	}

	if len(words) > 3 {
		half := len(words) / 2
		queries = append(queries,
			SearchQuery{Query: strings.Join(words[:half], " ")},
			SearchQuery{Query: strings.Join(words[half:], " ")},
		)
	}

	return queries
}

// --- refinement helpers ---

func broaden(q SearchQuery) SearchQuery {
	words := significantWords(q.Query)
	if len(words) <= 2 {
		return q
	}
	// drop the most specific (last) word to broaden
	return SearchQuery{
		Query:      strings.Join(words[:len(words)-1], " "),
		MaxResults: q.MaxResults,
		SafeSearch: q.SafeSearch,
		Language:   q.Language,
		Categories: q.Categories,
	}
}

func narrow(q SearchQuery, original string) SearchQuery {
	query := q.Query
	if !strings.Contains(strings.ToLower(query), strings.ToLower(original)) && len(original) < 100 {
		query = query + " " + extractKeyTerm(original)
	}
	return SearchQuery{
		Query:      strings.TrimSpace(query),
		MaxResults: q.MaxResults,
		SafeSearch: q.SafeSearch,
		Language:   q.Language,
		Categories: q.Categories,
	}
}

func hasLowRelevance(resp SearchResponse) bool {
	if len(resp.Results) == 0 {
		return false
	}
	lowCount := 0
	for _, r := range resp.Results {
		if r.Metadata.Score > 0 && r.Metadata.Score < 0.3 {
			lowCount++
		}
	}
	return lowCount > len(resp.Results)/2
}

func extractCoveredTerms(resp SearchResponse) []string {
	var terms []string
	for _, r := range resp.Results {
		for _, w := range significantWords(r.Title) {
			terms = append(terms, strings.ToLower(w))
		}
	}
	return terms
}

func findGaps(original string, coveredTerms []string) []string {
	covered := make(map[string]bool, len(coveredTerms))
	for _, t := range coveredTerms {
		covered[t] = true
	}

	var gaps []string
	for _, w := range significantWords(original) {
		if !covered[strings.ToLower(w)] {
			gaps = append(gaps, w)
		}
	}

	if len(gaps) == 0 {
		return nil
	}

	return []string{strings.Join(gaps, " ")}
}

func refinementHints(intent SearchIntent) []string {
	switch intent {
	case IntentComparison:
		return []string{"add benchmark data", "check recent reviews"}
	case IntentHowTo:
		return []string{"verify version compatibility", "check for common pitfalls"}
	case IntentNews:
		return []string{"narrow time range if too many results"}
	case IntentReview:
		return []string{"look for verified purchaser reviews", "check multiple sources"}
	default:
		return nil
	}
}

// --- text utilities ---

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"for": true, "and": true, "but": true, "or": true, "nor": true,
	"not": true, "so": true, "yet": true, "both": true, "either": true,
	"neither": true, "each": true, "every": true, "all": true, "any": true,
	"few": true, "more": true, "most": true, "other": true,
	"some": true, "such": true, "no": true, "only": true, "own": true,
	"same": true, "than": true, "too": true, "very": true,
	"of": true, "in": true, "to": true, "on": true, "at": true,
	"by": true, "with": true, "from": true, "up": true, "about": true,
	"into": true, "through": true, "during": true, "before": true,
	"after": true, "above": true, "below": true, "between": true,
	"out": true, "off": true, "over": true, "under": true,
	"i": true, "me": true, "my": true, "we": true, "our": true,
	"you": true, "your": true, "he": true, "him": true, "his": true,
	"she": true, "her": true, "it": true, "its": true, "they": true,
	"them": true, "their": true, "what": true, "which": true, "who": true,
	"whom": true, "this": true, "that": true, "these": true, "those": true,
	"how": true, "where": true, "when": true, "why": true,
	"find": true, "get": true, "give": true, "go": true, "make": true,
	"know": true, "take": true, "come": true, "think": true, "look": true,
	"want": true, "use": true, "tell": true, "also": true,
}

func significantWords(text string) []string {
	words := strings.Fields(text)
	var sig []string
	for _, w := range words {
		lower := strings.ToLower(strings.Trim(w, ".,!?;:\"'()[]{}"))
		if lower != "" && !stopWords[lower] {
			sig = append(sig, lower)
		}
	}
	return sig
}

func extractKeyTerm(text string) string {
	words := significantWords(text)
	if len(words) == 0 {
		return ""
	}
	longest := words[0]
	for _, w := range words[1:] {
		if len(w) > len(longest) {
			longest = w
		}
	}
	return longest
}

func extractComparisonSubjects(lower string) []string {
	for _, sep := range []string{" vs ", " versus ", " or ", " compared to ", " against "} {
		if idx := strings.Index(lower, sep); idx > 0 {
			left := strings.TrimSpace(lower[:idx])
			right := strings.TrimSpace(lower[idx+len(sep):])

			left = trimComparisonPrefix(left)
			right = trimComparisonSuffix(right)

			if left != "" && right != "" {
				return []string{left, right}
			}
		}
	}
	return nil
}

func trimComparisonPrefix(s string) string {
	for _, prefix := range []string{"compare ", "which is better ", "what is better "} {
		s = strings.TrimPrefix(s, prefix)
	}
	return strings.TrimSpace(s)
}

func trimComparisonSuffix(s string) string {
	for _, suffix := range []string{" for me", " for us", " to use"} {
		s = strings.TrimSuffix(s, suffix)
	}
	return strings.TrimSpace(s)
}
