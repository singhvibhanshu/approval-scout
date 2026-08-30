package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration, sourced entirely from environment
// variables so the tool is easy to drive from GitHub Actions secrets.
type Config struct {
	// Target repository, in "owner/name" form.
	Owner string
	Repo  string

	// GitHub auth. A token is strongly recommended (higher rate limits and,
	// if it has read:org, optional team-membership expansion). In GitHub
	// Actions the built-in secrets.GITHUB_TOKEN is enough for public reads.
	Token string

	// Scanning behaviour.
	IncludeDrafts bool
	ExpandTeams   bool     // resolve team membership (needs a token with read:org)
	IgnoreOwners  []string // owners to ignore, e.g. "@open-telemetry/collector-contrib-approvers"
	Concurrency   int

	// Reporting.
	Timezone      string
	SendWhenEmpty bool
	DryRun        bool // print the report to stdout instead of emailing

	// SMTP / email delivery.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	EmailFrom    string
	EmailTo      []string
}

// LoadConfig reads configuration from the environment and validates it.
func LoadConfig() (*Config, error) {
	c := &Config{
		IncludeDrafts: envBool("INCLUDE_DRAFTS", false),
		ExpandTeams:   envBool("EXPAND_TEAMS", false),
		IgnoreOwners:  envList("IGNORE_OWNERS"),
		Concurrency:   envInt("CONCURRENCY", 8),
		Timezone:      envStr("REPORT_TIMEZONE", "Asia/Kolkata"),
		SendWhenEmpty: envBool("SEND_WHEN_EMPTY", false),
		DryRun:        envBool("DRY_RUN", false),
		Token:         firstNonEmpty(os.Getenv("GH_PAT"), os.Getenv("GITHUB_TOKEN")),

		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     envInt("SMTP_PORT", 587),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		EmailFrom:    envStr("EMAIL_FROM", os.Getenv("SMTP_USERNAME")),
		EmailTo:      envList("EMAIL_TO"),
	}

	target := envStr("TARGET_REPO", "open-telemetry/opentelemetry-collector-contrib")
	parts := strings.SplitN(strings.TrimSpace(target), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("TARGET_REPO must be in owner/name form, got %q", target)
	}
	c.Owner, c.Repo = parts[0], parts[1]

	if c.Concurrency < 1 {
		c.Concurrency = 1
	}

	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return nil, fmt.Errorf("invalid REPORT_TIMEZONE %q: %w", c.Timezone, err)
	}

	if !c.DryRun {
		if c.SMTPHost == "" {
			return nil, fmt.Errorf("SMTP_HOST is required (or set DRY_RUN=true to print the report)")
		}
		if len(c.EmailTo) == 0 {
			return nil, fmt.Errorf("EMAIL_TO is required (comma-separated recipients)")
		}
		if c.EmailFrom == "" {
			return nil, fmt.Errorf("EMAIL_FROM (or SMTP_USERNAME) is required")
		}
	}

	return c, nil
}

// Location returns the configured reporting timezone (validated in LoadConfig).
func (c *Config) Location() *time.Location {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// envList parses a comma-separated env var into a trimmed, non-empty slice.
func envList(key string) []string {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
