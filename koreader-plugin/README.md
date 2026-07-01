# Ink Dashboard KOReader Plugin

Install:

```text
inkdash.koplugin -> /mnt/us/koreader/plugins/inkdash.koplugin
```

Optional file-based API key:

```text
/mnt/us/koreader/plugins/inkdash.koplugin/apikey.txt
```

Optional file-based defaults:

```text
/mnt/us/koreader/plugins/inkdash.koplugin/config.lua
```

Example:

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

`config.lua` overrides the built-in defaults. Settings saved from the KOReader UI still win over `config.lua`.

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

The plugin also supports configuration inside KOReader:

```text
Tools -> Ink Dashboard -> Configure
```

Menu actions:

- `Start dashboard`: starts auto-refresh and displays the dashboard.
- `Fetch dashboard now`: fetches one screen.
- `Configure`: edits API key, base URL, refresh interval, and orientation.
- `Enable auto-refresh`: keeps refreshing in the background.
- `Use server refresh interval`: lets the backend control refresh timing.
- `Use KOReader local clock`: switches the top-left clock between local and server rendering.
- `Enable live minute clock`: wakes once per minute to update the local clock region.
- `E-ink refresh type`: choose KOReader's screen refresh mode; `UI (balanced)` is recommended for live minute clock color consistency.
