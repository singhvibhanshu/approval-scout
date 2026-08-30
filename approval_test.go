package main

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hmarr/codeowners"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestLatestApprovers(t *testing.T) {
	tests := []struct {
		name    string
		reviews []review
		want    []string
	}{
		{
			name: "single approval",
			reviews: []review{
				{Login: "Alice", State: "APPROVED", SubmittedAt: ts("2026-01-01T10:00:00Z")},
			},
			want: []string{"alice"},
		},
		{
			name: "comment after approval keeps approval",
			reviews: []review{
				{Login: "alice", State: "APPROVED", SubmittedAt: ts("2026-01-01T10:00:00Z")},
				{Login: "alice", State: "COMMENTED", SubmittedAt: ts("2026-01-02T10:00:00Z")},
			},
			want: []string{"alice"},
		},
		{
			name: "changes requested after approval drops it",
			reviews: []review{
				{Login: "alice", State: "APPROVED", SubmittedAt: ts("2026-01-01T10:00:00Z")},
				{Login: "alice", State: "CHANGES_REQUESTED", SubmittedAt: ts("2026-01-02T10:00:00Z")},
			},
			want: nil,
		},
		{
			name: "re-approval after changes requested counts",
			reviews: []review{
				{Login: "alice", State: "CHANGES_REQUESTED", SubmittedAt: ts("2026-01-01T10:00:00Z")},
				{Login: "alice", State: "APPROVED", SubmittedAt: ts("2026-01-03T10:00:00Z")},
			},
			want: []string{"alice"},
		},
		{
			name: "dismissed approval does not count",
			reviews: []review{
				{Login: "bob", State: "APPROVED", SubmittedAt: ts("2026-01-01T10:00:00Z")},
				{Login: "bob", State: "DISMISSED", SubmittedAt: ts("2026-01-02T10:00:00Z")},
			},
			want: nil,
		},
		{
			name: "multiple reviewers",
			reviews: []review{
				{Login: "alice", State: "APPROVED", SubmittedAt: ts("2026-01-01T10:00:00Z")},
				{Login: "bob", State: "COMMENTED", SubmittedAt: ts("2026-01-01T11:00:00Z")},
				{Login: "carol", State: "APPROVED", SubmittedAt: ts("2026-01-01T12:00:00Z")},
			},
			want: []string{"alice", "carol"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := latestApprovers(tt.reviews)
			var keys []string
			for k := range got {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			sort.Strings(tt.want)
			if strings.Join(keys, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("latestApprovers = %v, want %v", keys, tt.want)
			}
		})
	}
}

const sampleCodeowners = `* @open-telemetry/collector-contrib-approvers
receiver/hostmetricsreceiver/   @open-telemetry/collector-contrib-approvers @alice @bob
exporter/kafkaexporter/         @open-telemetry/collector-contrib-approvers @carol
`

func parseRuleset(t *testing.T) codeowners.Ruleset {
	t.Helper()
	rs, err := codeowners.ParseFile(strings.NewReader(sampleCodeowners))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return rs
}

func TestOwnersOfFiles(t *testing.T) {
	rs := parseRuleset(t)

	// Ignore the umbrella team so only component owners remain.
	ignore := []string{"@open-telemetry/collector-contrib-approvers"}
	logins, teams := ownersOfFiles(rs, []string{
		"receiver/hostmetricsreceiver/scraper.go",
		"exporter/kafkaexporter/factory.go",
	}, ignore)

	want := map[string]bool{"alice": true, "bob": true, "carol": true}
	if len(logins) != len(want) {
		t.Fatalf("logins = %v, want %v", logins, want)
	}
	for k := range want {
		if !logins[k] {
			t.Fatalf("expected login %q in %v", k, logins)
		}
	}
	if len(teams) != 0 {
		t.Fatalf("expected umbrella team ignored, got teams %v", teams)
	}
}

func TestOwnersOfFilesTeamKept(t *testing.T) {
	rs := parseRuleset(t)
	// Without an ignore list, the umbrella team is retained.
	_, teams := ownersOfFiles(rs, []string{"README.md"}, nil)
	if !teams["open-telemetry/collector-contrib-approvers"] {
		t.Fatalf("expected umbrella team, got %v", teams)
	}
}

func TestEndToEndMatch(t *testing.T) {
	rs := parseRuleset(t)
	ignore := []string{"@open-telemetry/collector-contrib-approvers"}

	files := []string{"receiver/hostmetricsreceiver/scraper.go"}
	approvers := latestApprovers([]review{
		{Login: "bob", State: "APPROVED", SubmittedAt: ts("2026-01-01T10:00:00Z")},
		{Login: "randomcontributor", State: "APPROVED", SubmittedAt: ts("2026-01-01T11:00:00Z")},
	})

	ownerLogins, _ := ownersOfFiles(rs, files, ignore)
	matched := intersect(approvers, ownerLogins)
	if len(matched) != 1 || matched[0] != "bob" {
		t.Fatalf("matched = %v, want [bob]", matched)
	}

	set := map[string]bool{"bob": true}
	comps := matchedComponents(rs, files, set)
	if len(comps) != 1 || comps[0] != "receiver/hostmetricsreceiver/" {
		t.Fatalf("components = %v, want [receiver/hostmetricsreceiver/]", comps)
	}
}
