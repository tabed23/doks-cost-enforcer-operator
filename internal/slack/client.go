// Package slack posts cost alerts to a Slack incoming webhook.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const httpTimeout = 10 * time.Second

// ── Slack payload types ───────────────────────────────────────────────────────

type payload struct {
	Attachments []attachment `json:"attachments"`
}

type attachment struct {
	Color  string  `json:"color"`
	Blocks []block `json:"blocks"`
}

type block struct {
	Type   string   `json:"type"`
	Text   *mrkdwn  `json:"text,omitempty"`
	Fields []mrkdwn `json:"fields,omitempty"`
}

type mrkdwn struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ── AlertParams ───────────────────────────────────────────────────────────────

// AlertParams carries everything needed to build a Slack message.
type AlertParams struct {
	PolicyName           string
	PoolID               string
	NodeSize             string
	NodeCount            int
	HourlyCostPerNode    float64
	MonthlyCap           float64
	EstimatedMonthlyCost float64
	PercentOfCap         float64
}

// ── Public senders ────────────────────────────────────────────────────────────

// SendWarning fires the 80% threshold warning.
func SendWarning(ctx context.Context, webhookURL string, a AlertParams) error {
	return send(ctx, webhookURL, buildThresholdAlert(a,
		"warning",
		"#FFA500",
		":warning: *DOKS cost warning*",
		fmt.Sprintf("Pool `%s` is at *%.1f%%* of the $%.0f/mo cap.",
			a.PoolID, a.PercentOfCap, a.MonthlyCap),
	))
}

// SendCritical fires the 100% threshold critical alert.
func SendCritical(ctx context.Context, webhookURL string, a AlertParams) error {
	return send(ctx, webhookURL, buildThresholdAlert(a,
		"critical",
		"#E53E3E",
		":rotating_light: *DOKS cost cap reached*",
		fmt.Sprintf("Pool `%s` has *exceeded* the $%.0f/mo cap — currently at *%.1f%%*.",
			a.PoolID, a.MonthlyCap, a.PercentOfCap),
	))
}

// SendDailyDigest sends the 24h cost summary regardless of threshold.
func SendDailyDigest(ctx context.Context, webhookURL string, a AlertParams) error {
	color := "#36A64F" // green
	switch {
	case a.PercentOfCap >= 100:
		color = "#E53E3E"
	case a.PercentOfCap >= 80:
		color = "#FFA500"
	}

	p := payload{
		Attachments: []attachment{{
			Color: color,
			Blocks: []block{
				{
					Type: "section",
					Text: &mrkdwn{Type: "mrkdwn", Text: ":bar_chart: *DOKS daily cost digest*"},
				},
				{
					Type: "section",
					Fields: []mrkdwn{
						{Type: "mrkdwn", Text: fmt.Sprintf("*Policy*\n`%s`", a.PolicyName)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Pool ID*\n`%s`", a.PoolID)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Node size*\n`%s`", a.NodeSize)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Node count*\n%d", a.NodeCount)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Hourly / node*\n$%.4f", a.HourlyCostPerNode)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Monthly cap*\n$%.2f", a.MonthlyCap)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Est. monthly cost*\n$%.2f", a.EstimatedMonthlyCost)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*%% of cap*\n%.1f%%", a.PercentOfCap)},
					},
				},
			},
		}},
	}
	return send(ctx, webhookURL, p)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func buildThresholdAlert(a AlertParams, level, color, title, summary string) payload {
	return payload{
		Attachments: []attachment{{
			Color: color,
			Blocks: []block{
				{
					Type: "section",
					Text: &mrkdwn{
						Type: "mrkdwn",
						Text: fmt.Sprintf("%s\n%s", title, summary),
					},
				},
				{
					Type: "section",
					Fields: []mrkdwn{
						{Type: "mrkdwn", Text: fmt.Sprintf("*Policy*\n`%s`", a.PolicyName)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Level*\n%s", level)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Nodes*\n%d × `%s`", a.NodeCount, a.NodeSize)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Hourly / node*\n$%.4f", a.HourlyCostPerNode)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Est. cost*\n$%.2f / mo", a.EstimatedMonthlyCost)},
						{Type: "mrkdwn", Text: fmt.Sprintf("*Cap*\n$%.2f / mo", a.MonthlyCap)},
					},
				},
			},
		}},
	}
}

func send(ctx context.Context, webhookURL string, p payload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}