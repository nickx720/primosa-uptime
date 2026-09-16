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
	"net/url"
	"os"
	"strconv"
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
	CheckedAt    string                 `json:"checked_at"`
	Targets      map[string]TargetState `json:"targets"`
	LastUpdateID int64                  `json:"last_update_id"`
}

type checkResult struct {
	ok      bool
	detail  string
	latency time.Duration
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

func checkOnce(targetURL string) checkResult {
	start := time.Now()
	resp, err := httpClient.Get(targetURL)
	latency := time.Since(start)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return checkResult{ok: false, detail: "timeout", latency: latency}
		}
		return checkResult{ok: false, detail: err.Error(), latency: latency}
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	return checkResult{ok: ok, detail: fmt.Sprintf("status %d", resp.StatusCode), latency: latency}
}

func checkTarget(target Target) checkResult {
	first := checkOnce(target.URL)
	if first.ok {
		return first
	}
	time.Sleep(retryDelay)
	return checkOnce(target.URL)
}

// sendTelegram sends a standalone (non-reply) message.
func sendTelegram(text string) {
	sendTelegramMessage(text, 0)
}

// sendTelegramMessage sends text to TELEGRAM_CHAT_ID, optionally as a reply
// to replyToMessageID (0 means no reply). Falls back to printing when the
// Telegram env vars aren't set (local dry-run).
func sendTelegramMessage(text string, replyToMessageID int64) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		fmt.Printf("[dry-run] Telegram message: %s\n", text)
		return
	}

	payload := map[string]any{"chat_id": chatID, "text": text}
	if replyToMessageID != 0 {
		payload["reply_to_message_id"] = replyToMessageID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Telegram send failed: %v\n", err)
		return
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := httpClient.Post(endpoint, "application/json", bytes.NewReader(body))
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

// telegramMessage and telegramUpdate mirror the small subset of the
// Telegram Bot API's getUpdates response shape this tool needs.
type telegramMessage struct {
	MessageID int64 `json:"message_id"`
	Chat      struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text string `json:"text"`
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramUpdatesResponse struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

// getUpdates polls Telegram for updates with update_id >= offset. It uses
// timeout=0 (no long-polling — this runs once per 5-minute cron tick, not
// as a persistent process) and restricts to message updates only.
func getUpdates(offset int64) ([]telegramUpdate, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	q := url.Values{}
	q.Set("offset", strconv.FormatInt(offset, 10))
	q.Set("timeout", "0")
	q.Set("limit", "100")
	q.Set("allowed_updates", `["message"]`)
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?%s", token, q.Encode())

	resp, err := httpClient.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("getUpdates failed: %d %s", resp.StatusCode, buf.String())
	}

	var parsed telegramUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Result, nil
}

// isStatusCommand reports whether a message text is a /status command,
// with or without the @primosa_uptime_bot suffix Telegram appends in
// group chats with multiple bots.
func isStatusCommand(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "/status")
}

// findStatusCommand scans updates for the latest /status message in
// chatID, and separately tracks the highest update_id seen across ALL
// updates (regardless of chat or match) so the caller can always advance
// past them — Telegram never re-delivers an update once it's been
// acknowledged via a higher offset.
func findStatusCommand(updates []telegramUpdate, chatID int64) (msg *telegramMessage, maxUpdateID int64, found bool) {
	for _, u := range updates {
		if u.UpdateID > maxUpdateID {
			maxUpdateID = u.UpdateID
		}
		if u.Message == nil || u.Message.Chat.ID != chatID {
			continue
		}
		if isStatusCommand(u.Message.Text) {
			msg = u.Message
			found = true
		}
	}
	return msg, maxUpdateID, found
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

// formatDurationHM renders an ISO since-timestamp as "up for"/"down for"
// text with a coarser format than formatDuration: "Nm" under an hour, else
// "Nh 0Mm" (minutes zero-padded), matching the /status reply example.
func formatDurationHM(sinceISO string, now time.Time) string {
	since, err := time.Parse(time.RFC3339, sinceISO)
	if err != nil {
		return "unknown"
	}
	d := now.Sub(since).Round(time.Minute)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours <= 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh %02dm", hours, minutes)
}

// formatStatusReply builds the /status reply: one business-level line per
// target using this run's fresh results (status, latency) and state
// (since, for the up/down-for duration).
func formatStatusReply(targets []Target, state State, results map[string]checkResult, now time.Time) string {
	lines := []string{fmt.Sprintf("\U0001F4CA Status — %s", now.Format("2006-01-02 15:04 UTC"))}
	for _, t := range targets {
		ts := state.Targets[t.Name]
		result := results[t.Name]
		duration := formatDurationHM(ts.Since, now)

		if ts.Status == "down" {
			lines = append(lines, fmt.Sprintf("❌ %s — down (%s) · down for %s", t.Name, shortDownReason(result.detail), duration))
			continue
		}
		lines = append(lines, fmt.Sprintf("✅ %s — up · %d ms · up for %s", t.Name, result.latency.Milliseconds(), duration))
	}
	return strings.Join(lines, "\n")
}

// pollStatusCommand checks for a /status command posted to the Telegram
// group since the last processed update, and if found replies once with
// this run's status summary. It always advances state.LastUpdateID past
// whatever updates it saw, even when no command matched, so old messages
// are never re-answered.
func pollStatusCommand(state *State, targets []Target, results map[string]checkResult, now time.Time) {
	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	if os.Getenv("TELEGRAM_BOT_TOKEN") == "" || chatIDStr == "" {
		return
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Telegram status poll skipped: invalid TELEGRAM_CHAT_ID: %v\n", err)
		return
	}

	updates, err := getUpdates(state.LastUpdateID + 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Telegram getUpdates failed: %v\n", err)
		return
	}

	msg, maxUpdateID, found := findStatusCommand(updates, chatID)
	if maxUpdateID > 0 {
		state.LastUpdateID = maxUpdateID
	}
	if found {
		sendTelegramMessage(formatStatusReply(targets, *state, results, now), msg.MessageID)
	}
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

	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339)
	state.CheckedAt = now

	var firstSeen []firstSeenEntry
	results := map[string]checkResult{}

	// Drop state for targets no longer in targets.json so they don't linger.
	wanted := map[string]bool{}
	for _, target := range targets {
		wanted[target.Name] = true
	}
	for name := range state.Targets {
		if !wanted[name] {
			delete(state.Targets, name)
		}
	}

	for _, target := range targets {
		result := checkTarget(target)
		results[target.Name] = result
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

	pollStatusCommand(&state, targets, results, nowTime)

	if err := saveState(state); err != nil {
		fmt.Fprintf(os.Stderr, "Uptime check failed unexpectedly: %v\n", err)
	}
}

func main() {
	run()
	os.Exit(0)
}
