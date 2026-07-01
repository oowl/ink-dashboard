# Ink Dashboard

A personal e-ink dashboard for a jailbroken Kindle running KOReader.

This project has two parts:

- `koreader-plugin/inkdash.koplugin`: KOReader plugin that fetches and displays a full-screen PNG.
- `server`: Go backend that serves a TRMNL-like display API and renders dashboard PNGs.

## How It Works

1. KOReader calls `GET /api/display`.
2. The plugin sends device headers such as `png-width`, `png-height`, battery, and orientation.
3. The Go server renders an SVG dashboard for that size and converts it to PNG with `rsvg-convert`.
4. The server returns an image URL plus a refresh interval.
5. The plugin downloads the PNG, displays it full-screen, and schedules the next refresh.

## Plugin File Configuration

To override plugin defaults without editing `main.lua`, copy:

```text
koreader-plugin/inkdash.koplugin/config.lua.example
```

to:

```text
/mnt/us/koreader/plugins/inkdash.koplugin/config.lua
```

Then edit it:

```lua
return {
    api_key = "change-me",
    base_url = "http://192.168.1.20:8787",
    refresh_interval = 600,
    use_server_refresh_rate = true,
    refresh_type = "ui",
    orientation = "auto",
    show_notifications = true,
    local_clock = true,
    local_clock_live_minutes = false,
    local_clock_language = "auto",
    image_cache_seconds = -1,
}
```

Important: `config.lua` changes the default values. If you already saved settings through the KOReader UI, the saved UI settings take precedence. Use the UI to change current settings, or remove the saved `inkdash.lua` from KOReader's settings directory to return to file defaults.

`local_clock = true` lets KOReader render the top-left clock locally so the
server-rendered PNG does not change just because the time changed. By default it
updates only when the dashboard refreshes to save battery. Set
`local_clock_live_minutes = true` only if you want KOReader to wake once per
minute and update the clock locally. Use
`local_clock_language = "zh-CN"` or `"en"` to force the clock date language, or
leave `"auto"` to use the server language.

The local clock uses the same background and text colors as the server renderer.
When live minute updates are enabled, those minute updates use `refresh_type`
for the clock region. Keep `refresh_type = "ui"` for the best balance of color
consistency and battery life. `partial` is faster, but can leave visible gray
differences after minute updates on some Kindle panels.

`image_cache_seconds = -1` reuses the cached PNG while the server returns the
same content-hash filename. Set it to `0` only when debugging and you want to
redownload the PNG on every dashboard refresh.

## Run The Server

```bash
cd server
go run ./cmd/inkdash
```

The server automatically loads `.env` from the repository root or from `server/.env`. Shell environment variables still take precedence.

By default it listens on `0.0.0.0:8787` and uses API key `change-me`.

Admin dashboard:

```text
http://127.0.0.1:8787/admin
```

Use the same token as `INKDASH_API_KEY`. The admin page can save weather, calendar, display, refresh, and preview settings to `server/config.local.json`.
Private iCal URLs and weather tokens are stored there but are not echoed back into the admin form after saving; leave secret fields blank to keep existing values.

Smoke test:

```bash
curl -H 'access-token: change-me' \
  -H 'png-width: 600' \
  -H 'png-height: 800' \
  http://127.0.0.1:8787/api/display
```

Preview the SVG in a browser:

```text
http://127.0.0.1:8787/preview.svg?width=600&height=800&token=change-me
```

## Configure KOReader

Copy the plugin directory to your Kindle:

```text
koreader-plugin/inkdash.koplugin -> /mnt/us/koreader/plugins/inkdash.koplugin
```

Restart KOReader, then open:

```text
Tools -> Ink Dashboard -> Configure
```

Set:

- `API key`: same as `INKDASH_API_KEY`.
- `Base URL`: your server reachable from Kindle, for example `http://192.168.1.20:8787`.
- `Refresh interval`: fallback interval in seconds.
- `Orientation`: `auto`, `portrait`, `landscape`, or `rotated`.

Then choose:

```text
Tools -> Ink Dashboard -> Start dashboard
```

Tap the dashboard to exit interactive mode.

## Orientation

- `auto`: layout follows the requested canvas; `800x600` becomes landscape.
- `portrait`: portrait-style layout.
- `landscape`: landscape-style layout.
- `rotated`: returns a portrait-sized PNG whose content is rotated 90 degrees, useful when KOReader still reports `600x800` but you physically place the Kindle sideways.

## Current Data

The server now supports real weather and calendar data. If the related environment variables are missing or a request fails, it falls back to mock data.

Weather supports Open-Meteo and Caiyun. Open-Meteo needs only coordinates:

```bash
export INKDASH_WEATHER_PROVIDER=openmeteo
export INKDASH_WEATHER_LAT=31.2304
export INKDASH_WEATHER_LON=121.4737
export INKDASH_WEATHER_LOCATION=Shanghai
```

Caiyun needs a token:

```bash
export INKDASH_WEATHER_PROVIDER=caiyun
export INKDASH_CAIYUN_TOKEN='your-caiyun-token'
export INKDASH_CAIYUN_LANG=zh_CN
export INKDASH_CAIYUN_UNIT=metric
```

Google Calendar uses your private iCal URL:

```bash
export INKDASH_CALENDAR_ICS_URL='https://calendar.google.com/calendar/ical/.../basic.ics'
```

In Google Calendar, open calendar settings, go to **Integrate calendar**, and copy **Secret address in iCal format**. Treat this URL like a password; anyone with it can read that calendar.
In the admin dashboard, saved Calendar feeds are shown as host plus fingerprint only. You can select existing feeds to delete and paste one or more new private iCal URLs to add them.
Calendar feeds are cached in memory for 15 minutes by default; set `INKDASH_CALENDAR_CACHE_SECONDS` to tune that.

For multiple calendars:

```bash
export INKDASH_CALENDAR_ICS_URLS='https://calendar.google.com/.../basic.ics,https://calendar.google.com/.../basic.ics'
```

Codex usage is read from local Codex session JSONL. On each lookup the server
checks the newest session JSONL timestamp; if it is older than 5 minutes, the
server briefly starts Codex in a pseudo-terminal, sends `/status` to refresh the
local session data, then reads the structured `rate_limits` JSONL event:

```bash
export INKDASH_CODEX_ENABLED=true
export INKDASH_CODEX_CLI=codex
export INKDASH_CODEX_CWD=/home/owl/code/ink-dashboard
export INKDASH_CODEX_JSONL_ROOT=~/.codex/sessions
export INKDASH_CODEX_JSONL_STALE_SECONDS=300
export INKDASH_CODEX_CACHE_SECONDS=300
export INKDASH_CODEX_STATUS_WAIT_SECONDS=12
```

This uses the same local Codex login as your shell. It does not require an
OpenAI API key and does not ask a model to answer a prompt; it drives the CLI
slash command directly only when the local JSONL data is stale.

Mock/fallback data lives in:

```text
server/internal/dashboard/data.go
```

Codex is live when local Codex JSONL includes rate limit events.

## API Contract

Request:

```http
GET /api/display
access-token: change-me
png-width: 600
png-height: 800
orientation: auto
percent-charged: 85
```

Response:

```json
{
  "image_url": "http://host:8787/screens/inkdash-600x800-abc123.png",
  "filename": "inkdash-600x800-abc123.png",
  "refresh_rate": 600
}
```

The plugin uses `filename` to decide whether a screen changed, so the backend includes a content hash in the file name.
