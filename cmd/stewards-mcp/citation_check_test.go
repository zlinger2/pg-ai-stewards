package main

import (
	"strings"
	"testing"
)

// These checks are deliberately database-free: checkCitation is the pure
// verification core. They prevent unresolved attribution from turning an
// unrelated whole-document occurrence into a positive verdict.
func TestCitationCheckScopeIntegrity(t *testing.T) {
	body := "Alpha section: the text appears here.\nBeta section: other words."
	split := strings.Index(body, "\n") + 1
	sections := []citationSection{
		{Ref: "s1", Heading: "Alpha", CharStart: 0, CharEnd: split},
		{Ref: "s2", Heading: "Beta", CharStart: split, CharEnd: len([]rune(body))},
	}
	quote := "the text appears here"

	cases := []struct {
		name     string
		in       CitationCheckInput
		sections []citationSection
		want     bool
		scope    string
	}{
		{"exact section", CitationCheckInput{Quote: quote, SectionRef: "s1"}, sections, true, "section s1"},
		{"wrong existing section", CitationCheckInput{Quote: quote, SectionRef: "s2"}, sections, false, "section s2"},
		{"missing section", CitationCheckInput{Quote: quote, SectionRef: "s9"}, sections, false, "unresolved section"},
		{"missing section map", CitationCheckInput{Quote: quote, SectionRef: "s1"}, nil, false, "unresolved section"},
		{"matching heading", CitationCheckInput{Quote: quote, Heading: "Alpha"}, sections, true, "section s1"},
		{"blank heading", CitationCheckInput{Quote: quote, Heading: "  "}, sections[:1], false, "unresolved heading"},
		{"missing heading", CitationCheckInput{Quote: quote, Heading: "Gamma"}, sections, false, "unresolved heading"},
		{"missing heading map", CitationCheckInput{Quote: quote, Heading: "Alpha"}, nil, false, "unresolved heading"},
		{"unscoped whole document", CitationCheckInput{Quote: quote}, sections, true, "whole document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkCitation(tc.in, "fixture", body, tc.sections)
			if got.Verified != tc.want {
				t.Fatalf("verified=%v, want %v: %+v", got.Verified, tc.want, got)
			}
			if !strings.Contains(got.Scope, tc.scope) {
				t.Fatalf("scope=%q, want substring %q", got.Scope, tc.scope)
			}
			if got.Verified && got.FoundAt == nil {
				t.Fatal("verified citations must include found_at")
			}
			if !got.Verified && got.FoundAt != nil {
				t.Fatal("unverified citations cannot include found_at")
			}
			if !tc.want && got.NearestExcerpt == "" {
				t.Fatal("negative verdict must retain diagnostic nearest excerpt")
			}
		})
	}
}

func TestCitationCheckNormalizedUnicodeOffsets(t *testing.T) {
	body := "前書き 🌍\n“CAFÉ”\u00a0—\n\taccord"
	quote := "\"café\" - accord"
	got := checkCitation(CitationCheckInput{Quote: quote}, "fixture", body, nil)
	if !got.Verified || got.FoundAt == nil {
		t.Fatalf("normalized quote did not verify: %+v", got)
	}
	if got.FoundAt.CharStart != 6 || got.FoundAt.CharEnd != len([]rune(body)) {
		t.Fatalf("incorrect character offsets: %+v", got.FoundAt)
	}
}

func TestCitationCheckAmbiguousHeadingMustResolveExactSection(t *testing.T) {
	body := "Policy: agreed text.\nPolicy addendum: other text."
	split := strings.Index(body, "\n") + 1
	sections := []citationSection{
		{Ref: "s1", Heading: "Policy", CharStart: 0, CharEnd: split},
		{Ref: "s2", Heading: "Policy addendum", CharStart: split, CharEnd: len([]rune(body))},
	}
	got := checkCitation(CitationCheckInput{Quote: "agreed text", Heading: "Policy"}, "fixture", body, sections)
	if got.Verified || got.FoundAt != nil {
		t.Fatalf("ambiguous heading wrongly verified: %+v", got)
	}
	if !strings.Contains(got.Scope, "ambiguous heading") || !strings.Contains(got.Note, "exact section_ref") {
		t.Fatalf("missing actionable ambiguity report: %+v", got)
	}
}

func TestCitationCheckVerifiesEntireLongQuote(t *testing.T) {
	prefix := strings.Repeat("é", citationQuoteCap)
	for _, tc := range []struct {
		name  string
		quote string
		want  bool
	}{
		{"complete match", prefix + " actual ending", true},
		{"invented ending", prefix + " invented ending", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "Intro. " + prefix + " actual ending"
			got := checkCitation(CitationCheckInput{Quote: tc.quote}, "fixture", body, nil)
			if got.Verified != tc.want {
				t.Fatalf("verified=%v, want %v", got.Verified, tc.want)
			}
			if tc.want && (got.FoundAt == nil || got.FoundAt.CharEnd != len([]rune(body))) {
				t.Fatalf("found_at must cover the entire quote: %+v", got.FoundAt)
			}
		})
	}
}

func TestCitationCheckDiagnosticsDoNotDenyTextOutsideScope(t *testing.T) {
	body := "Alpha: agreed text.\nBeta: other text."
	split := strings.Index(body, "\n") + 1
	sections := []citationSection{
		{Ref: "s1", Heading: "Alpha", CharStart: 0, CharEnd: split},
		{Ref: "s2", Heading: "Beta", CharStart: split, CharEnd: len([]rune(body))},
	}
	got := checkCitation(CitationCheckInput{Quote: "agreed text", SectionRef: "s2"}, "fixture", body, sections)
	if got.Verified || got.FoundAt != nil {
		t.Fatalf("wrong attribution verified: %+v", got)
	}
	if strings.Contains(got.Note, "does NOT appear") {
		t.Fatalf("diagnostic denies text present in source: %q", got.Note)
	}
	if got.NearestSectionRef != "s1" || got.OverlapChars != len("agreed text") {
		t.Fatalf("diagnostic did not locate the text: %+v", got)
	}
}
