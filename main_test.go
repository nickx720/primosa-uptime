package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestIsStatusCommand(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"/status", true},
		{"/status@primosa_uptime_bot", true},
		{"/status please", true},
		{"  /status  ", true},
		{"/statusfoo", true}, // prefix match is intentionally loose
		{"status", false},
		{"hello /status", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isStatusCommand(c.text); got != c.want {
			t.Errorf("isStatusCommand(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

// fakeUpdates unmarshals a JSON fixture in the same shape Telegram's
// getUpdates returns, without making any network call.
func fakeUpdates(t *testing.T, raw string) []telegramUpdate {
	t.Helper()
	var updates []telegramUpdate
	if err := json.Unmarshal([]byte(raw), &updates); err != nil {
		t.Fatalf("bad fixture JSON: %v", err)
	}
	return updates
}

func TestFindStatusCommand_PicksLatestMatchInTargetChat(t *testing.T) {
	const chatID = -100123456789
	updates := fakeUpdates(t, `[
		{"update_id": 10, "message": {"message_id": 1, "chat": {"id": -100123456789}, "text": "hello"}},
		{"update_id": 11, "message": {"message_id": 2, "chat": {"id": -999}, "text": "/status"}},
		{"update_id": 12, "message": {"message_id": 3, "chat": {"id": -100123456789}, "text": "/status"}},
		{"update_id": 13, "message": {"message_id": 4, "chat": {"id": -100123456789}, "text": "not a command"}},
		{"update_id": 14, "message": {"message_id": 5, "chat": {"id": -100123456789}, "text": "/status@primosa_uptime_bot"}}
	]`)

	msg, maxUpdateID, found := findStatusCommand(updates, chatID)
	if !found {
		t.Fatal("expected a /status command to be found")
	}
	if msg.MessageID != 5 {
		t.Errorf("expected the latest matching message (id 5), got id %d", msg.MessageID)
	}
	if maxUpdateID != 14 {
		t.Errorf("maxUpdateID = %d, want 14", maxUpdateID)
	}
}

func TestFindStatusCommand_NoMatchStillAdvancesOffset(t *testing.T) {
	const chatID = -100123456789
	updates := fakeUpdates(t, `[
		{"update_id": 20, "message": {"message_id": 1, "chat": {"id": -100123456789}, "text": "hello"}},
		{"update_id": 21, "message": {"message_id": 2, "chat": {"id": -999}, "text": "/status"}}
	]`)

	msg, maxUpdateID, found := findStatusCommand(updates, chatID)
	if found {
		t.Fatal("expected no /status command to be found in the target chat")
	}
	if msg != nil {
		t.Errorf("expected nil message, got %+v", msg)
	}
	if maxUpdateID != 21 {
		t.Errorf("maxUpdateID = %d, want 21 (must advance even with no match)", maxUpdateID)
	}
}

func TestFindStatusCommand_EmptyUpdates(t *testing.T) {
	msg, maxUpdateID, found := findStatusCommand(nil, -100123456789)
	if found || msg != nil || maxUpdateID != 0 {
		t.Errorf("expected zero values for empty updates, got msg=%+v maxUpdateID=%d found=%v", msg, maxUpdateID, found)
	}
}

func TestFormatDurationHM(t *testing.T) {
	now := time.Date(2026, 9, 16, 18, 40, 0, 0, time.UTC)
	cases := []struct {
		since string
		want  string
	}{
		{now.Add(-3*time.Hour - 12*time.Minute).Format(time.RFC3339), "3h 12m"},
		{now.Add(-1*time.Hour - 5*time.Minute).Format(time.RFC3339), "1h 05m"},
		{now.Add(-5 * time.Minute).Format(time.RFC3339), "5m"},
		{"not-a-timestamp", "unknown"},
	}
	for _, c := range cases {
		if got := formatDurationHM(c.since, now); got != c.want {
			t.Errorf("formatDurationHM(%q) = %q, want %q", c.since, got, c.want)
		}
	}
}

func TestFormatStatusReply(t *testing.T) {
	now := time.Date(2026, 9, 16, 18, 40, 0, 0, time.UTC)
	targets := []Target{
		{Name: "Proof of Life (staging)", URL: "https://pol-stg.primosa.ai/health"},
		{Name: "Proof of Life", URL: "https://pol.primosa.ai/health"},
		{Name: "Unsub", URL: "https://unsub.primosa.ai/healthz"},
	}
	state := State{Targets: map[string]TargetState{
		"Proof of Life (staging)": {Status: "up", Since: now.Add(-3*time.Hour - 12*time.Minute).Format(time.RFC3339)},
		"Proof of Life":           {Status: "down", Since: now.Add(-1*time.Hour - 5*time.Minute).Format(time.RFC3339)},
		"Unsub":                   {Status: "down", Since: now.Add(-1*time.Hour - 5*time.Minute).Format(time.RFC3339)},
	}}
	results := map[string]checkResult{
		"Proof of Life (staging)": {ok: true, detail: "status 200", latency: 212 * time.Millisecond},
		"Proof of Life":           {ok: false, detail: "status 404"},
		"Unsub":                   {ok: false, detail: "timeout"},
	}

	got := formatStatusReply(targets, state, results, now)
	want := strings.Join([]string{
		"\U0001F4CA Status — 2026-09-16 18:40 UTC",
		"✅ Proof of Life (staging) — up · 212 ms · up for 3h 12m",
		"❌ Proof of Life — down (404) · down for 1h 05m",
		"❌ Unsub — down (timeout) · down for 1h 05m",
	}, "\n")

	if got != want {
		t.Errorf("formatStatusReply mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
