// Package sensitivity provides pattern-based classification of text content
// for sensitivity levels (PII, internal IP, credentials). No LLM calls;
// classification is synchronous and adds <1ms to retrieval latency.
package sensitivity

import (
	"regexp"
	"strings"
)

// Level represents the sensitivity classification of a piece of content.
type Level int

const (
	None        Level = 0 // No sensitive content detected
	PII         Level = 1 // Email addresses, phone numbers, names
	InternalIP  Level = 2 // Internal docs, pricing, roadmaps
	Credentials Level = 3 // API keys, tokens, passwords
)

// String returns a human-readable label for the sensitivity level.
func (l Level) String() string {
	switch l {
	case None:
		return "none"
	case PII:
		return "pii"
	case InternalIP:
		return "internal"
	case Credentials:
		return "credentials"
	default:
		return "unknown"
	}
}

// Match describes a single pattern match found during classification.
type Match struct {
	Pattern string `json:"pattern"`
	Level   Level  `json:"level"`
}

// Result is the output of classifying a piece of text.
type Result struct {
	Level   Level   `json:"level"`
	Matches []Match `json:"matches,omitempty"`
}

// Config holds classifier configuration.
type Config struct {
	// InternalDomains are domain suffixes treated as internal (e.g. ".internal", ".corp").
	InternalDomains []string
}

// DefaultConfig returns a config with common internal domain patterns.
func DefaultConfig() Config {
	return Config{
		InternalDomains: []string{".internal", ".corp", ".local"},
	}
}

// Classifier detects sensitive content in text using compiled regex patterns.
type Classifier struct {
	cfg      Config
	patterns []pattern
}

type pattern struct {
	name  string
	re    *regexp.Regexp
	level Level
}

// Built-in patterns compiled once at construction time.
var builtinPatterns = []struct {
	name  string
	expr  string
	level Level
}{
	// Credentials — checked first (highest severity)
	{"aws_access_key", `AKIA[0-9A-Z]{16}`, Credentials},
	{"openai_api_key", `sk-[a-zA-Z0-9_-]{20,}`, Credentials},
	{"github_token", `ghp_[a-zA-Z0-9]{36}`, Credentials},
	{"github_token_old", `gh[pousr]_[a-zA-Z0-9]{36}`, Credentials},
	{"slack_token", `xox[baprs]-[a-zA-Z0-9-]+`, Credentials},
	{"generic_secret", `(?i)(password|secret|token|api_key|apikey)\s*[:=]\s*\S+`, Credentials},

	// PII
	{"email_address", `[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`, PII},
	{"phone_number", `(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`, PII},
	{"credit_card", `\b(?:\d[ -]*?){13,19}\b`, PII},
	{"ssn", `\b\d{3}-\d{2}-\d{4}\b`, PII},
}

// New creates a Classifier with the given config.
func New(cfg Config) *Classifier {
	c := &Classifier{cfg: cfg}
	for _, bp := range builtinPatterns {
		c.patterns = append(c.patterns, pattern{
			name:  bp.name,
			re:    regexp.MustCompile(bp.expr),
			level: bp.level,
		})
	}
	return c
}

// Classify scans text for sensitive patterns and returns the result.
// The returned Level is the highest severity found across all matches.
func (c *Classifier) Classify(text string) Result {
	var matches []Match
	maxLevel := None

	for _, p := range c.patterns {
		if p.re.MatchString(text) {
			matches = append(matches, Match{Pattern: p.name, Level: p.level})
			if p.level > maxLevel {
				maxLevel = p.level
			}
		}
	}

	// Check for internal domain references
	lower := strings.ToLower(text)
	for _, domain := range c.cfg.InternalDomains {
		if strings.Contains(lower, domain) {
			matches = append(matches, Match{Pattern: "internal_domain", Level: InternalIP})
			if InternalIP > maxLevel {
				maxLevel = InternalIP
			}
			break
		}
	}

	return Result{Level: maxLevel, Matches: matches}
}

// ClassifyBatch classifies multiple texts and returns the highest level
// across all of them, along with per-text results for any that matched.
func (c *Classifier) ClassifyBatch(texts []string) (Level, []Result) {
	maxLevel := None
	results := make([]Result, len(texts))
	for i, text := range texts {
		results[i] = c.Classify(text)
		if results[i].Level > maxLevel {
			maxLevel = results[i].Level
		}
	}
	return maxLevel, results
}
