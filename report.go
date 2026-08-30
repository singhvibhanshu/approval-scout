package main

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// ApprovedPR is one PR that has been approved by a code owner of its changed files.
type ApprovedPR struct {
	Number     int
	Title      string
	URL        string
	Author     string
	Approvers  []string // code-owner logins whose approval qualified this PR
	Components  []string // CODEOWNERS patterns the approvers own that were changed
	ChangedN   int      // number of changed files
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Report is the full result of a scan, ready to render.
type Report struct {
	Owner     string
	Repo      string
	TotalOpen int
	PRs       []ApprovedPR
	Generated time.Time // in the reporting timezone
	TZName    string
}

// Subject returns the email subject line.
func (r *Report) Subject() string {
	return fmt.Sprintf("[%s/%s] %d PR(s) approved by code owners - %s",
		r.Owner, r.Repo, len(r.PRs), r.Generated.Format("2006-01-02 15:04 MST"))
}

func (p ApprovedPR) approversDisplay() string {
	at := make([]string, len(p.Approvers))
	for i, a := range p.Approvers {
		at[i] = "@" + a
	}
	return strings.Join(at, ", ")
}

func (p ApprovedPR) componentsDisplay() string {
	if len(p.Components) == 0 {
		return "-"
	}
	return strings.Join(p.Components, ", ")
}

// RenderText renders a plain-text version of the report (email fallback + logs).
func (r *Report) RenderText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", r.Subject())
	fmt.Fprintf(&b, "Generated %s (%s). %d of %d open PRs are approved by a code owner.\n\n",
		r.Generated.Format("Mon 2006-01-02 15:04:05 MST"), r.TZName, len(r.PRs), r.TotalOpen)

	if len(r.PRs) == 0 {
		b.WriteString("No open PRs currently have a code-owner approval.\n")
		return b.String()
	}
	for i, pr := range r.PRs {
		fmt.Fprintf(&b, "%d. #%d %s\n", i+1, pr.Number, pr.Title)
		fmt.Fprintf(&b, "   %s\n", pr.URL)
		fmt.Fprintf(&b, "   author: @%s | approved by: %s\n", pr.Author, pr.approversDisplay())
		fmt.Fprintf(&b, "   components: %s | files changed: %d | updated: %s\n\n",
			pr.componentsDisplay(), pr.ChangedN, pr.UpdatedAt.In(r.Generated.Location()).Format("2006-01-02 15:04 MST"))
	}
	return b.String()
}

var htmlTmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"approvers":  func(p ApprovedPR) string { return p.approversDisplay() },
	"components": func(p ApprovedPR) string { return p.componentsDisplay() },
	"localTime": func(t time.Time, ref time.Time) string {
		return t.In(ref.Location()).Format("2006-01-02 15:04 MST")
	},
}).Parse(htmlBody))

// RenderHTML renders the HTML email body.
func (r *Report) RenderHTML() (string, error) {
	var buf bytes.Buffer
	if err := htmlTmpl.Execute(&buf, r); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const htmlBody = `<!doctype html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>
<body style="margin:0;background:#f6f8fa;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;color:#1f2328;">
  <div style="max-width:760px;margin:0 auto;padding:24px 16px;">
    <h2 style="margin:0 0 4px;font-size:20px;">Code-owner approved PRs</h2>
    <p style="margin:0 0 16px;color:#59636e;font-size:14px;">
      <a href="https://github.com/{{.Owner}}/{{.Repo}}/pulls" style="color:#0969da;text-decoration:none;">{{.Owner}}/{{.Repo}}</a>
      &middot; {{len .PRs}} of {{.TotalOpen}} open PRs &middot; generated {{.Generated.Format "Mon 2006-01-02 15:04 MST"}}
    </p>
    {{if not .PRs}}
      <p style="padding:16px;background:#fff;border:1px solid #d0d7de;border-radius:8px;font-size:14px;">
        No open PRs currently have a code-owner approval.
      </p>
    {{else}}
      {{range $i, $pr := .PRs}}
      <div style="background:#fff;border:1px solid #d0d7de;border-radius:8px;padding:14px 16px;margin-bottom:10px;">
        <div style="font-size:15px;font-weight:600;line-height:1.4;">
          <a href="{{$pr.URL}}" style="color:#0969da;text-decoration:none;">#{{$pr.Number}}</a>
          <span style="color:#1f2328;">{{$pr.Title}}</span>
        </div>
        <div style="margin-top:6px;font-size:13px;color:#59636e;">
          <span style="display:inline-block;margin-right:14px;">author <b style="color:#1f2328;">@{{$pr.Author}}</b></span>
          <span style="display:inline-block;margin-right:14px;">approved by <b style="color:#1a7f37;">{{approvers $pr}}</b></span>
          <span style="display:inline-block;margin-right:14px;">{{$pr.ChangedN}} file(s)</span>
          <span style="display:inline-block;">updated {{localTime $pr.UpdatedAt $.Generated}}</span>
        </div>
        <div style="margin-top:6px;font-size:12px;color:#59636e;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;">
          {{components $pr}}
        </div>
      </div>
      {{end}}
    {{end}}
    <p style="margin-top:18px;color:#8c959f;font-size:12px;">
      Sent by <a href="https://github.com/singhvibhanshu/approval-scout" style="color:#8c959f;">ApprovalScout</a>.
    </p>
  </div>
</body>
</html>`
