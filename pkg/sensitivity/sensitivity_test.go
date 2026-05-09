package sensitivity

import (
	"strings"
	"testing"
)

func TestClassify_None(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("The service uses REST APIs for communication.")
	if r.Level != None {
		t.Errorf("expected None, got %s", r.Level)
	}
	if len(r.Matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(r.Matches))
	}
}

func TestClassify_Email(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("Contact alice@example.com for details.")
	if r.Level != PII {
		t.Errorf("expected PII, got %s", r.Level)
	}
	found := false
	for _, m := range r.Matches {
		if m.Pattern == "email_address" {
			found = true
		}
	}
	if !found {
		t.Error("expected email_address pattern match")
	}
}

func TestClassify_PhoneNumber(t *testing.T) {
	c := New(DefaultConfig())
	tests := []string{
		"Call me at (555) 123-4567",
		"Phone: +1-555-123-4567",
		"Reach out: 555.123.4567",
	}
	for _, text := range tests {
		r := c.Classify(text)
		if r.Level < PII {
			t.Errorf("expected PII for %q, got %s", text, r.Level)
		}
	}
}

func TestClassify_CreditCard(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("Card: 4111 1111 1111 1111")
	if r.Level != PII {
		t.Errorf("expected PII, got %s", r.Level)
	}
}

func TestClassify_SSN(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("SSN: 123-45-6789")
	if r.Level != PII {
		t.Errorf("expected PII, got %s", r.Level)
	}
}

func TestClassify_AWSKey(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("AWS key: AKIAIOSFODNN7EXAMPLE")
	if r.Level != Credentials {
		t.Errorf("expected Credentials, got %s", r.Level)
	}
	found := false
	for _, m := range r.Matches {
		if m.Pattern == "aws_access_key" {
			found = true
		}
	}
	if !found {
		t.Error("expected aws_access_key pattern match")
	}
}

func TestClassify_OpenAIKey(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("key: sk-proj-abc123def456ghi789jkl012mno345")
	if r.Level != Credentials {
		t.Errorf("expected Credentials, got %s", r.Level)
	}
}

func TestClassify_GitHubToken(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij")
	if r.Level != Credentials {
		t.Errorf("expected Credentials, got %s", r.Level)
	}
}

func TestClassify_SlackToken(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("slack: xoxb-123456789-abcdefghij")
	if r.Level != Credentials {
		t.Errorf("expected Credentials, got %s", r.Level)
	}
}

func TestClassify_GenericSecret(t *testing.T) {
	c := New(DefaultConfig())
	tests := []string{
		"password: hunter2",
		"API_KEY=sk_live_abc123",
		"secret: mysecretvalue",
		"token = abc123def456",
	}
	for _, text := range tests {
		r := c.Classify(text)
		if r.Level != Credentials {
			t.Errorf("expected Credentials for %q, got %s", text, r.Level)
		}
	}
}

func TestClassify_InternalDomain(t *testing.T) {
	c := New(DefaultConfig())
	r := c.Classify("See docs at wiki.internal for the architecture overview.")
	if r.Level != InternalIP {
		t.Errorf("expected InternalIP, got %s", r.Level)
	}
	found := false
	for _, m := range r.Matches {
		if m.Pattern == "internal_domain" {
			found = true
		}
	}
	if !found {
		t.Error("expected internal_domain pattern match")
	}
}

func TestClassify_CustomInternalDomain(t *testing.T) {
	c := New(Config{InternalDomains: []string{".acme.co"}})
	r := c.Classify("Check pricing at sales.acme.co/q3-proposal")
	if r.Level != InternalIP {
		t.Errorf("expected InternalIP, got %s", r.Level)
	}
}

func TestClassify_HighestLevelWins(t *testing.T) {
	c := New(DefaultConfig())
	// Text with both PII and credentials — credentials should win
	r := c.Classify("Contact alice@example.com, key: AKIAIOSFODNN7EXAMPLE")
	if r.Level != Credentials {
		t.Errorf("expected Credentials (highest), got %s", r.Level)
	}
	if len(r.Matches) < 2 {
		t.Errorf("expected at least 2 matches, got %d", len(r.Matches))
	}
}

func TestClassifyBatch(t *testing.T) {
	c := New(DefaultConfig())
	texts := []string{
		"Normal text with no sensitive data",
		"Email: bob@company.com",
		"AWS: AKIAIOSFODNN7EXAMPLE",
	}
	maxLevel, results := c.ClassifyBatch(texts)
	if maxLevel != Credentials {
		t.Errorf("expected batch max Credentials, got %s", maxLevel)
	}
	if results[0].Level != None {
		t.Errorf("expected first text None, got %s", results[0].Level)
	}
	if results[1].Level != PII {
		t.Errorf("expected second text PII, got %s", results[1].Level)
	}
	if results[2].Level != Credentials {
		t.Errorf("expected third text Credentials, got %s", results[2].Level)
	}
}

func TestClassifyBatch_Empty(t *testing.T) {
	c := New(DefaultConfig())
	maxLevel, results := c.ClassifyBatch(nil)
	if maxLevel != None {
		t.Errorf("expected None for empty batch, got %s", maxLevel)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{None, "none"},
		{PII, "pii"},
		{InternalIP, "internal"},
		{Credentials, "credentials"},
		{Level(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestClassify_NoFalsePositive_ShortNumbers(t *testing.T) {
	c := New(DefaultConfig())
	// Short numbers should not trigger credit card detection
	r := c.Classify("There are 42 items in the list.")
	for _, m := range r.Matches {
		if m.Pattern == "credit_card" {
			t.Error("short number should not match credit_card pattern")
		}
	}
}

// Benchmark: classification must add <1ms per the issue requirements.
func BenchmarkClassify_Short(b *testing.B) {
	c := New(DefaultConfig())
	text := "The auth service uses JWT with RS256 signing."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Classify(text)
	}
}

func BenchmarkClassify_WithMatches(b *testing.B) {
	c := New(DefaultConfig())
	text := "Contact alice@example.com, key: AKIAIOSFODNN7EXAMPLE, see wiki.internal"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Classify(text)
	}
}

func BenchmarkClassify_LongText(b *testing.B) {
	c := New(DefaultConfig())
	// ~2000 chars of realistic context
	text := strings.Repeat("The authentication service uses JWT tokens with RS256 signing. It validates tokens on every request. ", 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Classify(text)
	}
}

func BenchmarkClassifyBatch_10(b *testing.B) {
	c := New(DefaultConfig())
	texts := make([]string, 10)
	for i := range texts {
		texts[i] = "The service processes user data and connects to internal systems at api.internal for auth."
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.ClassifyBatch(texts)
	}
}
