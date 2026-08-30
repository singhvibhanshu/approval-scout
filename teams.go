package main

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/google/go-github/v84/github"
)

// teamResolver looks up (and caches) the member logins of GitHub teams. It is
// best-effort: team membership is only readable with a token that has the
// read:org scope and access to the org, so any failure is logged once and the
// team is treated as having no resolvable members.
type teamResolver struct {
	gh      *github.Client
	mu      sync.Mutex
	cache   map[string]map[string]bool // "org/slug" -> set of member logins (lower-cased)
	warned  bool
	enabled bool
}

func newTeamResolver(gh *github.Client, enabled bool) *teamResolver {
	return &teamResolver{gh: gh, cache: map[string]map[string]bool{}, enabled: enabled}
}

// isMember reports whether login belongs to the team identified by "org/slug".
func (t *teamResolver) isMember(ctx context.Context, orgSlug, login string) bool {
	if !t.enabled {
		return false
	}
	members := t.members(ctx, orgSlug)
	return members[strings.ToLower(login)]
}

func (t *teamResolver) members(ctx context.Context, orgSlug string) map[string]bool {
	t.mu.Lock()
	if m, ok := t.cache[orgSlug]; ok {
		t.mu.Unlock()
		return m
	}
	t.mu.Unlock()

	m := t.fetchMembers(ctx, orgSlug)

	t.mu.Lock()
	t.cache[orgSlug] = m
	t.mu.Unlock()
	return m
}

func (t *teamResolver) fetchMembers(ctx context.Context, orgSlug string) map[string]bool {
	parts := strings.SplitN(orgSlug, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	org, slug := parts[0], parts[1]

	members := map[string]bool{}
	opt := &github.TeamListTeamMembersOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		users, resp, err := t.gh.Teams.ListTeamMembersBySlug(ctx, org, slug, opt)
		if err != nil {
			t.warnOnce("could not resolve team %s (team expansion needs a token with read:org): %v", orgSlug, err)
			return members
		}
		for _, u := range users {
			members[strings.ToLower(u.GetLogin())] = true
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return members
}

func (t *teamResolver) warnOnce(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.warned {
		return
	}
	t.warned = true
	log.Printf("[warn] "+format, args...)
}
