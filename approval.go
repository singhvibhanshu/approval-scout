package main

import (
	"strings"
	"time"

	"github.com/hmarr/codeowners"
)

// review is a minimal, provider-agnostic view of a PR review, used so the
// core approval logic can be unit-tested without the GitHub client.
type review struct {
	Login       string
	State       string // APPROVED, CHANGES_REQUESTED, DISMISSED, COMMENTED, ...
	SubmittedAt time.Time
}

// latestApprovers returns the set of reviewer logins (lower-cased) whose most
// recent *stateful* review is an approval.
//
// This mirrors GitHub's own behaviour: a COMMENTED review never changes a
// reviewer's approval state, so it is ignored. An APPROVED review that is later
// DISMISSED (or followed by CHANGES_REQUESTED) no longer counts.
func latestApprovers(reviews []review) map[string]bool {
	type latest struct {
		state string
		at    time.Time
	}
	byUser := map[string]latest{}

	for _, r := range reviews {
		login := strings.ToLower(strings.TrimSpace(r.Login))
		if login == "" {
			continue
		}
		state := strings.ToUpper(strings.TrimSpace(r.State))
		// COMMENTED and PENDING reviews do not affect approval state.
		if state == "COMMENTED" || state == "PENDING" || state == "" {
			continue
		}
		cur, ok := byUser[login]
		if !ok || r.SubmittedAt.After(cur.at) {
			byUser[login] = latest{state: state, at: r.SubmittedAt}
		}
	}

	approvers := map[string]bool{}
	for login, l := range byUser {
		if l.state == "APPROVED" {
			approvers[login] = true
		}
	}
	return approvers
}

// ownersOfFiles walks the changed file paths, matches each against the
// CODEOWNERS ruleset (last matching rule wins), and returns the distinct
// individual owner logins and team slugs that own at least one changed file.
//
// Owners appearing in ignore (matched by their "@..."/email display form,
// case-insensitively) are skipped - handy for dropping an umbrella team such as
// "@open-telemetry/collector-contrib-approvers" so the report focuses on
// component-level owners.
//
// The returned logins are lower-cased; teams are kept as "org/slug".
func ownersOfFiles(ruleset codeowners.Ruleset, files []string, ignore []string) (logins map[string]bool, teams map[string]bool) {
	logins = map[string]bool{}
	teams = map[string]bool{}

	ignored := map[string]bool{}
	for _, ig := range ignore {
		ignored[strings.ToLower(strings.TrimSpace(ig))] = true
	}

	for _, f := range files {
		rule, err := ruleset.Match(f)
		if err != nil || rule == nil {
			continue
		}
		for _, o := range rule.Owners {
			if ignored[strings.ToLower(o.String())] {
				continue
			}
			switch o.Type {
			case codeowners.UsernameOwner:
				logins[strings.ToLower(o.Value)] = true
			case codeowners.TeamOwner:
				teams[strings.ToLower(o.Value)] = true
			}
		}
	}
	return logins, teams
}

// matchedComponents returns the distinct CODEOWNERS patterns (e.g.
// "receiver/hostmetricsreceiver/") that a set of changed files fall under and
// that are owned by one of ownerLogins. This is used purely to make the report
// human-friendly ("which components did this owner sign off on").
func matchedComponents(ruleset codeowners.Ruleset, files []string, ownerLogins map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		rule, err := ruleset.Match(f)
		if err != nil || rule == nil {
			continue
		}
		owns := false
		for _, o := range rule.Owners {
			if o.Type == codeowners.UsernameOwner && ownerLogins[strings.ToLower(o.Value)] {
				owns = true
				break
			}
		}
		if !owns {
			continue
		}
		p := rule.RawPattern()
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// intersect returns the keys present in both sets (order unspecified).
func intersect(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if b[k] {
			out = append(out, k)
		}
	}
	return out
}
