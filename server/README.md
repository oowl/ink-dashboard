# Ink Dashboard Server

Go backend for the KOReader Ink Dashboard plugin.

## Run

```bash
go run ./cmd/inkdash
```

The server automatically loads `.env` from the repository root or `server/.env`. Existing shell environment variables take precedence over file values.

## Admin Dashboard

Open:

```text
http://127.0.0.1:8787/admin
```

Use `INKDASH_API_KEY` as the admin token. The admin dashboard writes runtime settings to:

```text
config.local.json
```

That file is ignored by git because it can contain private Calendar URLs and weather tokens.
The admin form does not echo saved iCal URLs or weather tokens back into HTML. Leave those fields blank to keep existing secrets, or use the clear checkbox to remove them.

## Environment

See the root `.env.example`.

Important variables:

- `INKDASH_API_KEY`: token expected in `access-token`.
- `INKDASH_PUBLIC_BASE_URL`: externally reachable base URL for returned image URLs.
- `INKDASH_REFRESH_SECONDS`: server-provided refresh interval.
- `INKDASH_ORIENTATION`: `auto`, `portrait`, `landscape`, or `rotated`.
- `INKDASH_TIMEZONE`: display timezone, for example `Asia/Shanghai`.
- `INKDASH_LANGUAGE`: `zh-CN` or `en`, controls admin text and generated dashboard labels.
- `INKDASH_WEATHER_PROVIDER`: `openmeteo` or `caiyun`.
- `INKDASH_WEATHER_LAT` / `INKDASH_WEATHER_LON`: enable Open-Meteo weather.
- `INKDASH_WEATHER_LOCATION`: label shown on the dashboard.
- `INKDASH_CAIYUN_TOKEN`: Caiyun API token.
- `INKDASH_CAIYUN_LANG`: default `zh_CN`.
- `INKDASH_CAIYUN_UNIT`: default `metric`.
- `INKDASH_CALENDAR_ICS_URL`: Google Calendar private iCal URL.
- `INKDASH_CALENDAR_ICS_URLS`: comma-separated private iCal URLs for multiple calendars.
- `INKDASH_CALENDAR_LOOKAHEAD_DAYS`: how far ahead to scan events.
- `INKDASH_CALENDAR_MAX_EVENTS`: maximum events shown.
- `INKDASH_CALENDAR_MAX_BYTES`: max downloaded iCal feed size, default 25 MiB.
- `INKDASH_CALENDAR_CACHE_SECONDS`: in-memory iCal cache TTL, default 900 seconds.
- `INKDASH_CODEX_ENABLED`: enable local Codex usage lookup, default `true`.
- `INKDASH_CODEX_CLI`: Codex binary path, default `codex`.
- `INKDASH_CODEX_CWD`: working directory passed to `codex -C`, default `.`.
- `INKDASH_CODEX_MODEL`: optional model override for the lookup.
- `INKDASH_CODEX_JSONL_ROOT`: Codex session JSONL directory, default `~/.codex/sessions`.
- `INKDASH_CODEX_JSONL_STALE_SECONDS`: refresh JSONL with `/status` after this age, default 300 seconds.
- `INKDASH_CODEX_TIMEOUT_SECONDS`: Codex CLI timeout, default 60 seconds.
- `INKDASH_CODEX_CACHE_SECONDS`: retained for compatibility; JSONL mtime is the refresh gate.
- `INKDASH_CODEX_STATUS_WAIT_SECONDS`: time to leave `/status` on screen before closing the TUI, default 12 seconds.

## Google Calendar

Use Google Calendar's **Secret address in iCal format**:

1. Open Google Calendar in a browser.
2. Open Settings.
3. Under **Settings for my calendars**, choose the calendar.
4. Open **Integrate calendar**.
5. Copy **Secret address in iCal format**.

Put it in `.env` or export it in your shell. Do not commit it.
The admin dashboard can manage multiple private iCal feeds. Existing feeds are shown as host plus fingerprint, with checkboxes for deletion; paste new URLs one per line to add them.

## Codex Usage

Codex usage is read from local Codex session JSONL. If the newest session JSONL
is older than `INKDASH_CODEX_JSONL_STALE_SECONDS`, the server opens the local
CLI in a pseudo-terminal, sends `/status`, closes the TUI, then reads the
structured `rate_limits` event from JSONL:

```bash
codex -C "$INKDASH_CODEX_CWD"
# then the server sends /status and reads ~/.codex/sessions/*.jsonl
```

The server shows the most constrained remaining percentage across the 5-hour
and weekly Codex windows.

## Weather

Weather is fetched from Open-Meteo:

```bash
export INKDASH_WEATHER_LAT=31.2304
export INKDASH_WEATHER_LON=121.4737
export INKDASH_WEATHER_LOCATION=Shanghai
```

Or from Caiyun:

```bash
export INKDASH_WEATHER_PROVIDER=caiyun
export INKDASH_CAIYUN_TOKEN=your-caiyun-token
export INKDASH_WEATHER_LAT=31.2304
export INKDASH_WEATHER_LON=121.4737
export INKDASH_WEATHER_LOCATION=Shanghai
```

## Endpoints

- `GET /health`
- `GET /api/display` accepts `local-clock: true` to leave the top-left clock blank for KOReader-side rendering.
- `GET /preview.svg`
- `GET /screens/<filename>.png`
