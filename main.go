// Command approval-scout scans every open PR in a GitHub
// repository and reports those that have been approved by a code owner of the
// files they change, per the repository's CODEOWNERS file. It is built for
// triagers of open-telemetry/opentelemetry-collector-contrib but works against
// any repo with a CODEOWNERS file.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/go-github/v84/github"
)

func main() {
	log.SetFlags(log.Ltime)

	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	gh := github.NewClient(nil)
	if cfg.Token != "" {
		gh = gh.WithAuthToken(cfg.Token)
	} else {
		log.Printf("[warn] no GITHUB_TOKEN/GH_PAT set - using unauthenticated API (low rate limit)")
	}

	scanner, err := NewScanner(ctx, cfg, gh)
	if err != nil {
		return err
	}

	prs, totalOpen, err := scanner.Scan(ctx)
	if err != nil {
		return err
	}

	report := &Report{
		Owner:     cfg.Owner,
		Repo:      cfg.Repo,
		TotalOpen: totalOpen,
		PRs:       prs,
		Generated: time.Now().In(cfg.Location()),
		TZName:    cfg.Timezone,
	}

	log.Printf("found %d PR(s) approved by a code owner", len(prs))

	if cfg.DryRun {
		fmt.Println()
		fmt.Println(report.RenderText())
		return nil
	}

	if len(prs) == 0 && !cfg.SendWhenEmpty {
		log.Printf("no qualifying PRs and SEND_WHEN_EMPTY=false - skipping email")
		return nil
	}

	htmlBody, err := report.RenderHTML()
	if err != nil {
		return err
	}
	if err := sendEmail(cfg, report.Subject(), report.RenderText(), htmlBody); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}
	log.Printf("emailed report to %d recipient(s)", len(cfg.EmailTo))

	// Surface the summary in the Actions log / step summary too.
	if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
		if f, ferr := os.OpenFile(summaryPath, os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			fmt.Fprintf(f, "### %s\n\n```\n%s\n```\n", report.Subject(), report.RenderText())
			f.Close()
		}
	}
	return nil
}
