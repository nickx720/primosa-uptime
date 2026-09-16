// Free uptime monitor for *.primosa.ai projects. Node 22+, no deps.
// Reads targets.json, checks each with a retry to avoid flapping, diffs
// against state.json, sends a Telegram message on up<->down transitions,
// and writes state.json back. Always exits 0.

import { readFile, writeFile } from 'node:fs/promises';

const TIMEOUT_MS = 15_000;
const RETRY_DELAY_MS = 10_000;
const TARGETS_FILE = new URL('./targets.json', import.meta.url);
const STATE_FILE = new URL('./state.json', import.meta.url);

const { TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID } = process.env;

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function checkOnce(url) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS);
  try {
    const res = await fetch(url, { signal: controller.signal });
    return { ok: res.ok, detail: `status ${res.status}` };
  } catch (err) {
    return { ok: false, detail: err.name === 'AbortError' ? 'timeout' : String(err.message || err) };
  } finally {
    clearTimeout(timer);
  }
}

async function checkTarget(target) {
  const first = await checkOnce(target.url);
  if (first.ok) return first;
  await sleep(RETRY_DELAY_MS);
  const second = await checkOnce(target.url);
  return second;
}

async function sendTelegram(text) {
  if (!TELEGRAM_BOT_TOKEN || !TELEGRAM_CHAT_ID) {
    console.log(`[dry-run] Telegram message: ${text}`);
    return;
  }
  try {
    const url = `https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage`;
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ chat_id: TELEGRAM_CHAT_ID, text }),
    });
    if (!res.ok) {
      console.error(`Telegram send failed: ${res.status} ${await res.text()}`);
    }
  } catch (err) {
    console.error(`Telegram send failed: ${err.message || err}`);
  }
}

function formatDuration(sinceIso) {
  const ms = Date.now() - new Date(sinceIso).getTime();
  const minutes = Math.round(ms / 60_000);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.round(hours / 24);
  return `${days}d`;
}

async function main() {
  const targets = JSON.parse(await readFile(TARGETS_FILE, 'utf8'));
  let state = {};
  try {
    state = JSON.parse(await readFile(STATE_FILE, 'utf8'));
  } catch {
    state = {};
  }

  const now = new Date().toISOString();

  for (const target of targets) {
    const result = await checkTarget(target);
    const newStatus = result.ok ? 'up' : 'down';
    const prev = state[target.name];

    if (!prev) {
      state[target.name] = { status: newStatus, since: now };
      continue;
    }

    if (prev.status !== newStatus) {
      if (newStatus === 'down') {
        await sendTelegram(`🔴 ${target.name} is DOWN (${result.detail}) ${target.url}`);
      } else {
        const duration = formatDuration(prev.since);
        await sendTelegram(`🟢 ${target.name} is back UP after ${duration}`);
      }
      state[target.name] = { status: newStatus, since: now };
    }
    // unchanged: leave state as-is (keep original 'since')
  }

  await writeFile(STATE_FILE, JSON.stringify(state, null, 2) + '\n');
}

main()
  .catch((err) => {
    console.error('Uptime check failed unexpectedly:', err);
  })
  .finally(() => {
    process.exit(0);
  });
