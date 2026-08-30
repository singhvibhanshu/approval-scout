package main

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/hmarr/codeowners"
	"golang.org/x/sync/errgroup"
)

// Scanner encapsulates everything needed to scan a repository's open PRs for
// code-owner approvals.
type Scanner struct {
	cfg     *Config
	gh      *github.Client
	ruleset codeowners.Ruleset
	teams   *teamResolver
}

// NewScanner builds a Scanner, fetching and parsing the CODEOWNERS file up front.
func NewScanner(ctx context.Context, cfg *Config, gh *github.Client) (*Scanner, error) {
	ruleset, path, err := fetchCodeowners(ctx, gh, cfg.Owner, cfg.Repo)
	if err != nil {
		return nil, err
	}
	log.Printf("loaded CODEOWNERS from %s (%d rules)", path, len(ruleset))
	return &Scanner{
		cfg:     cfg,
		gh:      gh,
		ruleset: ruleset,
		teams:   newTeamResolver(gh, cfg.ExpandTeams),
	}, nil
}

// Scan returns the PRs approved by a code owner of their changed files, along
// with the total number of open PRs considered.
func (s *Scanner) Scan(ctx context.Context) ([]ApprovedPR, int, error) {
	prs, err := s.listOpenPRs(ctx)
	if err != nil {
		return nil, 0, err
	}
	log.Printf("scanning %d open PRs", len(prs))

	var (
		mu       sync.Mutex
		approved []ApprovedPR
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.cfg.Concurrency)

	for _, pr := range prs {
		pr := pr
		g.Go(func() error {
			res, err := s.evaluatePR(ctx, pr)
			if err != nil {
				return err
			}
			if res != nil {
				mu.Lock()
				approved = append(approved, *res)
				mu.Unlock()
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, err
	}

	// Most recently updated first - the PRs a triager most likely wants to act on.
	sort.Slice(approved, func(i, j int) bool {
		return approved[i].UpdatedAt.After(approved[j].UpdatedAt)
	})
	return approved, len(prs), nil
}

// evaluatePR returns a non-nil result if the PR has an approval from a code
// owner of at least one of its changed files.
func (s *Scanner) evaluatePR(ctx context.Context, pr *github.PullRequest) (*ApprovedPR, error) {
	// Cheap check first: does anyone currently approve this PR?
	rawReviews, err := s.listReviews(ctx, pr.GetNumber())
	if err != nil {
		return nil, err
	}
	reviews := make([]review, 0, len(rawReviews))
	for _, r := range rawReviews {
		reviews = append(reviews, review{
			Login:       r.GetUser().GetLogin(),
			State:       r.GetState(),
			SubmittedAt: r.GetSubmittedAt().Time,
		})
	}
	approvers := latestApprovers(reviews)
	if len(approvers) == 0 {
		return nil, nil
	}

	// Only now (for the minority of PRs with approvals) pay for the file list.
	files, err := s.listFiles(ctx, pr.GetNumber())
	if err != nil {
		return nil, err
	}
	ownerLogins, ownerTeams := ownersOfFiles(s.ruleset, files, s.cfg.IgnoreOwners)

	matched := map[string]bool{}
	// Direct individual-owner approvals.
	for _, login := range intersect(approvers, ownerLogins) {
		matched[login] = true
	}
	// Optional team-membership expansion.
	if s.cfg.ExpandTeams && len(ownerTeams) > 0 {
		for approver := range approvers {
			if matched[approver] {
				continue
			}
			for team := range ownerTeams {
				if s.teams.isMember(ctx, team, approver) {
					matched[approver] = true
					break
				}
			}
		}
	}
	if len(matched) == 0 {
		return nil, nil
	}

	approverList := make([]string, 0, len(matched))
	for login := range matched {
		approverList = append(approverList, login)
	}
	sort.Strings(approverList)

	return &ApprovedPR{
		Number:     pr.GetNumber(),
		Title:      pr.GetTitle(),
		URL:        pr.GetHTMLURL(),
		Author:     pr.GetUser().GetLogin(),
		Approvers:  approverList,
		Components: matchedComponents(s.ruleset, files, matched),
		ChangedN:   len(files),
		CreatedAt:  pr.GetCreatedAt().Time,
		UpdatedAt:  pr.GetUpdatedAt().Time,
	}, nil
}

func (s *Scanner) listOpenPRs(ctx context.Context) ([]*github.PullRequest, error) {
	opt := &github.PullRequestListOptions{
		State:       "open",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	var all []*github.PullRequest
	for {
		var (
			page []*github.PullRequest
			resp *github.Response
			err  error
		)
		if err = retry(ctx, func() error {
			page, resp, err = s.gh.PullRequests.List(ctx, s.cfg.Owner, s.cfg.Repo, opt)
			return err
		}); err != nil {
			return nil, err
		}
		for _, pr := range page {
			if pr.GetDraft() && !s.cfg.IncludeDrafts {
				continue
			}
			all = append(all, pr)
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return all, nil
}

func (s *Scanner) listReviews(ctx context.Context, number int) ([]*github.PullRequestReview, error) {
	opt := &github.ListOptions{PerPage: 100}
	var all []*github.PullRequestReview
	for {
		var (
			page []*github.PullRequestReview
			resp *github.Response
			err  error
		)
		if err = retry(ctx, func() error {
			page, resp, err = s.gh.PullRequests.ListReviews(ctx, s.cfg.Owner, s.cfg.Repo, number, opt)
			return err
		}); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return all, nil
}

func (s *Scanner) listFiles(ctx context.Context, number int) ([]string, error) {
	opt := &github.ListOptions{PerPage: 100}
	var all []string
	for {
		var (
			page []*github.CommitFile
			resp *github.Response
			err  error
		)
		if err = retry(ctx, func() error {
			page, resp, err = s.gh.PullRequests.ListFiles(ctx, s.cfg.Owner, s.cfg.Repo, number, opt)
			return err
		}); err != nil {
			return nil, err
		}
		for _, f := range page {
			all = append(all, f.GetFilename())
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return all, nil
}

// retry runs fn, backing off and retrying on GitHub rate-limit / secondary
// rate-limit errors up to a few times.
func retry(ctx context.Context, fn func() error) error {
	const maxAttempts = 4
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		wait, ok := rateLimitDelay(err)
		if !ok || attempt == maxAttempts {
			return err
		}
		log.Printf("[warn] rate limited, waiting %s before retry (attempt %d/%d)", wait.Round(time.Second), attempt, maxAttempts)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// rateLimitDelay reports how long to wait for a rate-limit error, if that's what
// err is.
func rateLimitDelay(err error) (time.Duration, bool) {
	var rle *github.RateLimitError
	if errors.As(err, &rle) {
		d := time.Until(rle.Rate.Reset.Time) + time.Second
		if d < time.Second {
			d = time.Second
		}
		if d > 5*time.Minute {
			d = 5 * time.Minute
		}
		return d, true
	}
	var abe *github.AbuseRateLimitError
	if errors.As(err, &abe) {
		if abe.RetryAfter != nil {
			return *abe.RetryAfter + time.Second, true
		}
		return 30 * time.Second, true
	}
	return 0, false
}
