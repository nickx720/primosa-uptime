// Command primosa-uptime is a free uptime monitor for *.primosa.ai projects.
// Node/deps-free by design: Go stdlib only. Reads targets.json, checks each
// with a retry to avoid flapping, diffs against state.json, sends a
// Telegram message on up<->down transitions, and writes state.json back.
// Always exits 0.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	timeout     = 15 * time.Second
	retryDelay  = 10 * time.Second
	targetsFile = "targets.json"
	stateFile   = "state.json"
)

type Target struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Expect int    `json:"expect"`
}

type TargetState struct {
	Status string `json:"status"`
	Since  string `json:"since"`
}

// State wraps per-target status plus a heartbeat timestamp that updates on
// every run (not just on a status change). Without this, a repo whose
// targets stay steady for 60+ days would produce no commits at all (see
// docs/003-approach-review.md) and GitHub would auto-disable the schedule.
type State struct {
	CheckedAt string                 `json:"checked_at"`
	Targets   map[string]TargetState `json:"targets"`
}

type checkResult struct {
	ok     bool
	detail string
}

// firstSeenEntry is a target with no prior state.json entry, collected
// during a run so a single summary message can be sent instead of one
// per target.
type firstSeenEntry struct {
	name   string
	status string
	detail string
}

var httpClient = &http.Client{Timeout: timeout}

func checkOnce(url string) checkResult {
	resp, err := httpClient.Get(url)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return checkResult{ok: false, detail: "timeout"}
		}
		return checkResult{ok: false, detail: err.Error()}
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	return checkResult{ok: ok, detail: fmt.Sprintf("status %d", resp.StatusCode)}
}

func checkTarget(target Target) checkResult {
	first := checkOnce(target.URL)
	if first.ok {
		return first
	}
	time.Sleep(retryDelay)
	return checkOnce(target.URL)
}

func sendTelegram(text string) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		fmt.Printf("[dry-run] Telegram message: %s\n", text)
		return
	}

	body, err := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Telegram send failed: %v\n", err)
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Telegram send failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		fmt.Fprintf(os.Stderr, "Telegram send failed: %d %s\n", resp.StatusCode, buf.String())
	}
}

// shortDownReason trims the "status " prefix off a check detail (e.g.
// "status 404" -> "404") so summary lines read "down (404)"; other
// details (e.g. "timeout", a raw error string) pass through unchanged.
func shortDownReason(detail string) string {
	if code, ok := strings.CutPrefix(detail, "status "); ok {
		return code
	}
	return detail
}

func formatStatusLine(e firstSeenEntry) string {
	if e.status == "down" {
		return fmt.Sprintf("%s — down (%s)", e.name, shortDownReason(e.detail))
	}
	return fmt.Sprintf("%s — up", e.name)
}

// formatFirstSeenSummary builds the one Telegram message covering every
// target seen for the first time this run. If every target is
// first-seen (a brand new repo/state.json) it uses the "started"
// header; otherwise (a target added later) it uses "now monitoring",
// collapsed to one line when only a single target is new.
func formatFirstSeenSummary(entries []firstSeenEntry, totalTargets int) string {
	allFirstSeen := len(entries) == totalTargets
	if len(entries) == 1 && !allFirstSeen {
		return fmt.Sprintf("\U0001F44B now monitoring %s", formatStatusLine(entries[0]))
	}

	header := "\U0001F44B now monitoring"
	if allFirstSeen {
		header = "\U0001F44B Uptime monitoring started"
	}
	lines := []string{header}
	for _, e := range entries {
		emoji := "✅"
		if e.status == "down" {
			emoji = "❌"
		}
		lines = append(lines, fmt.Sprintf("%s %s", emoji, formatStatusLine(e)))
	}
	return strings.Join(lines, "\n")
}

func formatDuration(sinceISO string) string {
	since, err := time.Parse(time.RFC3339, sinceISO)
	if err != nil {
		return "unknown"
	}
	minutes := int(time.Since(since).Round(time.Minute).Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(time.Since(since).Round(time.Hour).Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

func loadTargets() ([]Target, error) {
	data, err := os.ReadFile(targetsFile)
	if err != nil {
		return nil, err
	}
	var targets []Target
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func loadState() State {
	state := State{Targets: map[string]TargetState{}}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, &state)
	if state.Targets == nil {
		state.Targets = map[string]TargetState{}
	}
	return state
}

func saveState(state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(stateFile, data, 0o644)
}

func run() {
	targets, err := loadTargets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Uptime check failed unexpectedly: %v\n", err)
		return
	}
	state := loadState()
	if os.Getenv("UPTIME_RESET") != "" {
		// Treat every target as first-seen so the "now monitoring"
		// summary can be re-sent on demand.
		state = State{Targets: map[string]TargetState{}}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	state.CheckedAt = now

	var firstSeen []firstSeenEntry

	for _, target := range targets {
		result := checkTarget(target)
		newStatus := "down"
		if result.ok {
			newStatus = "up"
		}
		prev, seen := state.Targets[target.Name]

		if !seen {
			state.Targets[target.Name] = TargetState{Status: newStatus, Since: now}
			firstSeen = append(firstSeen, firstSeenEntry{name: target.Name, status: newStatus, detail: result.detail})
			continue
		}

		if prev.Status != newStatus {
			if newStatus == "down" {
				sendTelegram(fmt.Sprintf("\U0001F534 %s is DOWN (%s) %s", target.Name, result.detail, target.URL))
			} else {
				duration := formatDuration(prev.Since)
				sendTelegram(fmt.Sprintf("\U0001F7E2 %s is back UP after %s", target.Name, duration))
			}
			state.Targets[target.Name] = TargetState{Status: newStatus, Since: now}
		}
		// unchanged: leave state as-is (keep original Since)
	}

	if len(firstSeen) > 0 {
		sendTelegram(formatFirstSeenSummary(firstSeen, len(targets)))
	}

	if err := saveState(state); err != nil {
		fmt.Fprintf(os.Stderr, "Uptime check failed unexpectedly: %v\n", err)
	}
}

func main() {
	run()
	os.Exit(0)
}
