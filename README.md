# ApprovalScout

A small, self-hosted bot that scans **every open pull request** in a GitHub
repository and emails you the ones that have already been **approved by a code
owner of the files they change** - so a triager can go add the right labels
without hunting through the PR list by hand.

Built for triagers of
[`open-telemetry/opentelemetry-collector-contrib`](https://github.com/open-telemetry/opentelemetry-collector-contrib),
but it works against any repo that has a `CODEOWNERS` file.

- **Runs on a schedule** via GitHub Actions (default: 09:00 and 21:00 IST).
- **Iterates every open PR**, checks for an approval, and confirms the approver
  actually owns one of the PR's changed paths per `.github/CODEOWNERS`.
- **Emails an HTML report** with a direct link to each qualifying PR.
- Zero servers, zero database. Just a scheduled job and an SMTP account.

---

## How "approved by a code owner" is decided

This is the part that matters, so it's worth being precise:

1. List every **open** PR (drafts excluded by default).
2. For each PR, read its reviews and compute **each reviewer's latest review
   state**. This follows GitHub's own rules - a `COMMENTED` review never
   overrides an earlier `APPROVED`, and an approval that was later dismissed or
   followed by "changes requested" no longer counts.
3. Only for PRs that have at least one live approval, fetch the **changed
   files** and match each file against the `CODEOWNERS` patterns (last matching
   rule wins, exactly like GitHub).
4. The PR qualifies if an approver is an **individual owner** (`@username`) of at
   least one changed path.

In `opentelemetry-collector-contrib`, every `CODEOWNERS` line looks like:

```
receiver/hostmetricsreceiver/   @open-telemetry/collector-contrib-approvers @dmitryax @braydonk @rogercoll
```

The umbrella team `@open-telemetry/collector-contrib-approvers` is on *every*
line, so the meaningful signal is the **component owners** (`@dmitryax`, ...).
Matching those needs **no special permissions** - the built-in Actions token is
enough.

### Optional: team-membership expansion

Some paths are owned only by a team (e.g. `cmd/otelcontribcol/` ->
`@open-telemetry/collector-contrib-approvers` with no individuals). To count an
approval from *any member of an owning team*, set `EXPAND_TEAMS=true` and provide
a token with the `read:org` scope (a fine-grained or classic PAT). Without it,
team-only paths are simply not matched (logged, never fatal).

---

## Quick start (GitHub Actions - recommended)

1. **Fork or create a repo** from this project and push it to GitHub.
2. Add repository **Secrets** (Settings -> Secrets and variables -> Actions ->
   *Secrets*):

   | Secret          | Example                          | Notes |
   |-----------------|----------------------------------|-------|
   | `SMTP_HOST`     | `smtp.gmail.com`                 | |
   | `SMTP_PORT`     | `587`                            | `465` (implicit TLS) also works |
   | `SMTP_USERNAME` | `you@gmail.com`                  | |
   | `SMTP_PASSWORD` | *app password*                   | Gmail: [App Passwords](https://myaccount.google.com/apppasswords) |
   | `EMAIL_FROM`    | `you@gmail.com`                  | defaults to `SMTP_USERNAME` |
   | `EMAIL_TO`      | `you@gmail.com,mate@example.com` | comma-separated |
   | `GH_PAT`        | *(optional)* a PAT               | higher rate limit + team expansion |

3. *(Optional)* add repository **Variables** to tweak behaviour without editing
   code: `TARGET_REPO`, `INCLUDE_DRAFTS`, `EXPAND_TEAMS`, `IGNORE_OWNERS`,
   `SEND_WHEN_EMPTY`, `REPORT_TIMEZONE`.
4. Go to the **Actions** tab -> *approval-scout* -> **Run workflow**,
   tick **dry_run** the first time to see the report in the log before wiring up
   email. After that, the cron schedule takes over.

> GitHub disables scheduled workflows in a repo after 60 days of no commits, and
> may delay scheduled runs under load - both are GitHub platform behaviours, not
> this tool.

## Run it locally

```bash
cp .env.example .env      # fill in the values (DRY_RUN=true to just print)
make dry                  # prints the report to your terminal
make run                  # actually sends the email
make test                 # unit tests for the core matching logic
```

## Configuration reference

All configuration is via environment variables.

| Variable          | Default                                             | Description |
|-------------------|-----------------------------------------------------|-------------|
| `TARGET_REPO`     | `open-telemetry/opentelemetry-collector-contrib`    | `owner/name` to scan |
| `GH_PAT`          | -                                                   | GitHub token; falls back to `GITHUB_TOKEN` |
| `GITHUB_TOKEN`    | -                                                   | provided automatically inside Actions |
| `INCLUDE_DRAFTS`  | `false`                                             | include draft PRs |
| `EXPAND_TEAMS`    | `false`                                             | also count approvals from owning-team members (needs `read:org`) |
| `IGNORE_OWNERS`   | -                                                   | comma-separated owners to ignore, e.g. `@open-telemetry/collector-contrib-approvers` |
| `SEND_WHEN_EMPTY` | `false`                                             | send an email even when nothing qualifies |
| `REPORT_TIMEZONE` | `Asia/Kolkata`                                      | timezone shown in the report |
| `CONCURRENCY`     | `8`                                                 | parallel PR lookups |
| `DRY_RUN`         | `false`                                             | print the report instead of emailing |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` | - / `587` / - / - | SMTP delivery |
| `EMAIL_FROM` / `EMAIL_TO` | `SMTP_USERNAME` / -                         | sender / comma-separated recipients |

## Changing the schedule

Edit the `cron` line in [`.github/workflows/report.yml`](.github/workflows/report.yml).
Cron is in **UTC**. The defaults map to IST as:

| UTC     | IST      |
|---------|----------|
| `03:30` | 09:00    |
| `15:30` | 21:00    |

For a true every-8-hours cadence use `cron: "0 */8 * * *"`.

## How it stays within API rate limits

The tool fetches the changed-file list only for the *minority* of PRs that
actually have an approval, and backs off automatically on rate-limit responses.
A token (any token) gives you 5,000 requests/hour, which is plenty for a repo of
this size a few times a day.

## Notes & limitations

- Approval state mirrors GitHub's model but does **not** account for a review
  auto-dismissed by a new push unless GitHub itself recorded a `DISMISSED`
  event (it usually does).
- Team-only owned paths need `EXPAND_TEAMS=true` to match.
- This tool only *reports*; it does not add labels. That final click stays with
  you - by design, so a human confirms before labels move.

## License

Apache License 2.0 - see [LICENSE](LICENSE).
