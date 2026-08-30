package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v84/github"
	"github.com/hmarr/codeowners"
)

// codeownersLocations lists the paths GitHub recognises for a CODEOWNERS file,
// in priority order.
var codeownersLocations = []string{
	".github/CODEOWNERS",
	"CODEOWNERS",
	"docs/CODEOWNERS",
}

// fetchCodeowners downloads and parses the repository's CODEOWNERS file from the
// first standard location that exists on the default branch.
func fetchCodeowners(ctx context.Context, gh *github.Client, owner, repo string) (codeowners.Ruleset, string, error) {
	var lastErr error
	for _, path := range codeownersLocations {
		content, _, resp, err := gh.Repositories.GetContents(ctx, owner, repo, path, nil)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				continue
			}
			lastErr = err
			continue
		}
		if content == nil {
			continue
		}
		decoded, err := content.GetContent()
		if err != nil {
			return nil, "", fmt.Errorf("decoding %s: %w", path, err)
		}
		ruleset, err := codeowners.ParseFile(strings.NewReader(decoded))
		if err != nil {
			return nil, "", fmt.Errorf("parsing %s: %w", path, err)
		}
		return ruleset, path, nil
	}
	if lastErr != nil {
		return nil, "", fmt.Errorf("fetching CODEOWNERS: %w", lastErr)
	}
	return nil, "", fmt.Errorf("no CODEOWNERS file found in %s", strings.Join(codeownersLocations, ", "))
}
