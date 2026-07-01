package dashboard

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ink-dashboard/server/internal/config"
)

func TestReadLatestCodexJSONLUsage(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 8, 2, 0, 0, 0, loc)
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	resetPrimary := now.Add(30 * time.Minute).Unix()
	resetSecondary := now.Add(72 * time.Hour).Unix()
	content := `{"timestamp":"2026-06-07T18:07:41.163Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"limit_id":"codex","primary":{"used_percent":1.0,"window_minutes":300,"resets_at":` + strconvFormatInt(resetPrimary) + `},"secondary":{"used_percent":60.0,"window_minutes":10080,"resets_at":` + strconvFormatInt(resetSecondary) + `},"plan_type":"pro"}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := &RealProvider{
		cfg:      config.Config{Language: "zh-CN"},
		location: loc,
	}
	usage, err := provider.readCodexJSONLUsage(codexJSONLFile{Path: path}, now)
	if err != nil {
		t.Fatal(err)
	}

	if usage.Name != "Codex" || usage.Primary != "剩余 40%" || usage.Percent != 40 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("expected two Codex windows, got %#v", usage.Windows)
	}
	if usage.Windows[0].Label != "5小时" || usage.Windows[0].Percent != 99 || usage.Windows[1].Label != "每周" || usage.Windows[1].Percent != 40 {
		t.Fatalf("unexpected Codex windows: %#v", usage.Windows)
	}
	for _, want := range []string{"5小时 99% 重置02:30", "每周 40% 重置6月11日 02:00", "Pro"} {
		if !strings.Contains(usage.Secondary, want) {
			t.Fatalf("expected secondary to contain %q, got %q", want, usage.Secondary)
		}
	}
}

func TestLatestCodexJSONLPrefersSessions(t *testing.T) {
	root := t.TempDir()
	sessionPath := filepath.Join(root, "sessions", "2026", "06", "07", "rollout.jsonl")
	tmpPath := filepath.Join(root, ".tmp", "newer.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(sessionPath, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmpPath, time.Unix(200, 0), time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}

	latest, err := latestCodexJSONL(root)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Path != sessionPath {
		t.Fatalf("expected latest session path %q, got %q", sessionPath, latest.Path)
	}
}

func TestParseCodexStatus(t *testing.T) {
	raw := `
╭──────────────────────────────────────────────────────────────────────────────╮
│  >_ OpenAI Codex (v0.137.0)                                                  │
│  Account:                     user@example.com (Pro)                         │
│  5h limit:                    [█████████░░░░░░░░░░░] 47% left (resets 02:01) │
│  Weekly limit:                [████████░░░░░░░░░░░░] 40% left                │
│                               (resets 08:26 on 11 Jun)                       │
│  GPT-5.3-Codex-Spark limit:                                                  │
│  5h limit:                    [████████████████████] 100% left               │
╰──────────────────────────────────────────────────────────────────────────────╯`

	usage, err := parseCodexStatus(raw, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}

	if usage.Name != "Codex" || usage.Primary != "剩余 40%" || usage.Percent != 40 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("expected two Codex windows, got %#v", usage.Windows)
	}
	for _, want := range []string{"5小时 47% 重置02:01", "每周 40% 重置08:26 on 11 Jun", "Pro"} {
		if !strings.Contains(usage.Secondary, want) {
			t.Fatalf("expected secondary to contain %q, got %q", want, usage.Secondary)
		}
	}
	if strings.Contains(usage.Secondary, "example.com") || strings.Contains(usage.Secondary, "100%") {
		t.Fatalf("unexpected private or Spark block text in secondary: %q", usage.Secondary)
	}
}

func TestStripTerminalControlForStatus(t *testing.T) {
	raw := "\x1b[2m│  5h limit:\x1b[22m [bar] 47% left (resets 02:01) │\x1b[0m"
	usage, err := parseCodexStatus(raw, "en")
	if err != nil {
		t.Fatal(err)
	}
	if usage.Primary != "47% left" || usage.Percent != 47 {
		t.Fatalf("unexpected ANSI parse result: %#v", usage)
	}
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
