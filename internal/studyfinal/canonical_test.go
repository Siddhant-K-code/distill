package studyfinal

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestDigestDomainKnownAnswers(t *testing.T) {
	tests := []struct {
		domain string
		value  any
		want   string
	}{
		{"condition", map[string]any{"a": "NFC", "z": 1}, "8a4cf865911e197bdd0e84893e746844c4affd7d10df25216dab7a26afc3906b"},
		{"outcome", map[string]any{"identity_digest": strings.Repeat("0", 64), "result": "review"}, "4b791030da3696eed600d0dfa282dabb9a9184394ba68a5467fc0b271633dfef"},
		{"receipt", map[string]any{"attempt_count": 1, "timestamp": "2026-09-20T00:00:00Z"}, "56f80512bdbd5f9fb99d7fc55c17134752ea1c624f7ebf399885178e7dbcbf0d"},
	}
	for _, test := range tests {
		got, err := DigestDomain(test.domain, test.value)
		if err != nil || got != test.want {
			t.Fatalf("%s digest = %q, %v; want %q", test.domain, got, err, test.want)
		}
	}
}

func TestCanonicalJSONNormalizesNFCAndRejectsCollisions(t *testing.T) {
	composed, err := DigestDomain("condition", map[string]any{"name": "Caf\u00e9"})
	if err != nil {
		t.Fatal(err)
	}
	decomposed, err := DigestDomain("condition", map[string]any{"name": "Cafe\u0301"})
	if err != nil {
		t.Fatal(err)
	}
	if composed != decomposed {
		t.Fatal("canonically equivalent Unicode strings produced different digests")
	}
	if _, err := DigestDomain("condition", map[string]any{"\u00e9": 1, "e\u0301": 2}); err == nil {
		t.Fatal("NFC key collision was accepted")
	}
	if _, err := DigestDomain("condition", map[string]any{"number": math.Inf(1)}); err == nil {
		t.Fatal("non-finite number was accepted")
	}
}

func TestCanonicalJSONRFC8785NumbersAndOrdering(t *testing.T) {
	raw := []byte(`{"numbers":[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001],"values":{"\u20ac":"Euro Sign","\r":"CR","\ufb33":"Hebrew Letter Dalet With Dagesh","1":"One","\ud83d\ude00":"Emoji: Grinning Face","\u0080":"Control","\u00f6":"Latin Small Letter O With Diaeresis"}}`)
	value, err := decodeNormalizedJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := canonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("{\"numbers\":[333333333.3333333,1e+30,4.5,0.002,1e-27],\"values\":{\"\\r\":\"CR\",\"1\":\"One\",\"\u0080\":\"Control\",\"\u00f6\":\"Latin Small Letter O With Diaeresis\",\"דּ\":\"Hebrew Letter Dalet With Dagesh\",\"\u20ac\":\"Euro Sign\",\"😀\":\"Emoji: Grinning Face\"}}")
	if !bytes.Equal(got, want) {
		t.Fatalf("JCS output mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestStrictDecodeRejectsNestedDuplicatesAndTrailingData(t *testing.T) {
	var value map[string]any
	if err := strictDecode(strings.NewReader(`{"outer":{"value":1,"value":2}}`), &value); err == nil {
		t.Fatal("nested duplicate key was accepted")
	}
	if err := strictDecode(strings.NewReader("{\"value\":1}\n true"), &value); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
	if err := strictDecode(strings.NewReader("{\"value\":1}\n garbage"), &value); err == nil {
		t.Fatal("trailing non-whitespace data was accepted")
	}
}
