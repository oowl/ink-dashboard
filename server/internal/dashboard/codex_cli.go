package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ink-dashboard/server/internal/config"
)

type codexStatusLimit struct {
	Label string
	Left  int
	Reset string
}

type codexJSONLFile struct {
	Path    string
	ModTime time.Time
}

type codexJSONLRecord struct {
	Payload struct {
		RateLimits *codexJSONLRateLimits `json:"rate_limits"`
	} `json:"payload"`
}

type codexJSONLRateLimits struct {
	Primary   codexJSONLLimitWindow `json:"primary"`
	Secondary codexJSONLLimitWindow `json:"secondary"`
	PlanType  string                `json:"plan_type"`
}

type codexJSONLLimitWindow struct {
	UsedPercent  float64 `json:"used_percent"`
	WindowMinute int     `json:"window_minutes"`
	ResetsAt     int64   `json:"resets_at"`
}

func (p *RealProvider) FetchCodexUsage(ctx context.Context, now time.Time) (Usage, error) {
	if !p.cfg.CodexEnabled {
		if config.IsChinese(p.cfg.Language) {
			return Usage{Name: "Codex", Primary: "未启用", Secondary: "设置 INKDASH_CODEX_ENABLED=true", Percent: 0}, nil
		}
		return Usage{Name: "Codex", Primary: "disabled", Secondary: "set INKDASH_CODEX_ENABLED=true", Percent: 0}, nil
	}

	p.codexMu.Lock()
	defer p.codexMu.Unlock()

	latest, latestErr := latestCodexJSONL(p.cfg.CodexJSONLRoot)
	if latestErr == nil && !p.codexJSONLIsStale(latest, now) {
		p.cacheMu.Lock()
		cached := p.codex
		if cached.usage.Name != "" && cached.jsonlPath == latest.Path && cached.jsonlModTime.Equal(latest.ModTime) {
			p.cacheMu.Unlock()
			return cached.usage, cached.err
		}
		p.cacheMu.Unlock()

		usage, source, err := p.readCodexJSONLUsageFromRoot(now)
		if source.Path == "" {
			source = latest
		}
		return p.cacheCodexUsage(usage, source, now, err)
	}

	refreshErr := p.refreshCodexStatusCLI(ctx)
	refreshed, refreshedErr := latestCodexJSONL(p.cfg.CodexJSONLRoot)
	if refreshedErr != nil {
		if refreshErr != nil {
			refreshErr = fmt.Errorf("%w; latest codex jsonl: %v", refreshErr, refreshedErr)
		} else {
			refreshErr = refreshedErr
		}
		return p.cacheCodexUsage(Usage{}, codexJSONLFile{}, now, refreshErr)
	}

	usage, source, readErr := p.readCodexJSONLUsageFromRoot(now)
	if source.Path == "" {
		source = refreshed
	}
	if readErr == nil && refreshErr == nil {
		return p.cacheCodexUsage(usage, source, now, nil)
	}
	if readErr == nil {
		return p.cacheCodexUsage(usage, source, now, refreshErr)
	}
	if refreshErr != nil {
		readErr = fmt.Errorf("%w; codex jsonl read failed: %v", refreshErr, readErr)
	}
	return p.cacheCodexUsage(usage, source, now, readErr)
}

func (p *RealProvider) cacheCodexUsage(usage Usage, source codexJSONLFile, now time.Time, err error) (Usage, error) {
	p.cacheMu.Lock()
	if err != nil && usage.Name == "" && p.codex.usage.Name != "" {
		usage = p.codex.usage
	}
	if usage.Name == "" {
		usage = codexErrorUsage(p.cfg.Language)
	}
	p.codex = codexCacheEntry{
		usage:        usage,
		fetchedAt:    now,
		jsonlPath:    source.Path,
		jsonlModTime: source.ModTime,
		err:          err,
	}
	p.cacheMu.Unlock()

	return usage, err
}

func (p *RealProvider) codexJSONLIsStale(file codexJSONLFile, now time.Time) bool {
	staleAfter := time.Duration(p.cfg.CodexJSONLStaleSec) * time.Second
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	age := now.Sub(file.ModTime)
	return age < 0 || age >= staleAfter
}

func (p *RealProvider) refreshCodexStatusCLI(ctx context.Context) error {
	timeout := time.Duration(p.cfg.CodexTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	codexArgs := []string{p.cfg.CodexCLIPath}
	if strings.TrimSpace(p.cfg.CodexQueryCWD) != "" {
		codexArgs = append(codexArgs, "-C", strings.TrimSpace(p.cfg.CodexQueryCWD))
	}
	if strings.TrimSpace(p.cfg.CodexModel) != "" {
		codexArgs = append(codexArgs, "-m", strings.TrimSpace(p.cfg.CodexModel))
	}
	command := "stty rows 30 cols 100; " + shellCommand(codexArgs)

	cmd := exec.CommandContext(ctx, "script", "-qef", "-O", "/dev/stdout", "-c", command)
	if strings.TrimSpace(p.cfg.CodexQueryCWD) != "" {
		cmd.Dir = strings.TrimSpace(p.cfg.CodexQueryCWD)
	}
	cmd.Env = append(cmd.Environ(), "TERM=xterm-256color")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr

	wait := time.Duration(p.cfg.CodexStatusWaitSec) * time.Second
	if wait <= 0 {
		wait = 12 * time.Second
	}
	go func() {
		defer stdin.Close()
		time.Sleep(1 * time.Second)
		_, _ = stdin.Write([]byte("/status\r"))
		time.Sleep(wait)
		_, _ = stdin.Write([]byte{0x03})
	}()

	runErr := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("codex /status timed out after %s", timeout)
	}
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return nil
		}
		return fmt.Errorf("codex /status failed: %w: %s", runErr, shortCLIOutput(stderr.String()))
	}
	return nil
}

func (p *RealProvider) readCodexJSONLUsage(file codexJSONLFile, now time.Time) (Usage, error) {
	rateLimits, err := readLatestCodexRateLimits(file.Path)
	if err != nil {
		return Usage{}, err
	}
	return codexUsageFromRateLimits(rateLimits, p.location, now, p.cfg.Language), nil
}

func (p *RealProvider) readCodexJSONLUsageFromRoot(now time.Time) (Usage, codexJSONLFile, error) {
	rateLimits, source, err := readLatestCodexRateLimitsFromRoot(p.cfg.CodexJSONLRoot)
	if err != nil {
		return Usage{}, source, err
	}
	return codexUsageFromRateLimits(rateLimits, p.location, now, p.cfg.Language), source, nil
}

func latestCodexJSONL(root string) (codexJSONLFile, error) {
	files, err := codexJSONLFiles(root)
	if err != nil {
		return codexJSONLFile{}, err
	}
	if len(files) == 0 {
		return codexJSONLFile{}, fmt.Errorf("no codex jsonl files under %s", root)
	}
	return files[0], nil
}

func codexJSONLFiles(root string) ([]codexJSONLFile, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("codex jsonl root is empty")
	}
	var sessions []codexJSONLFile
	var any []codexJSONLFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		normalized := filepath.ToSlash(path)
		if strings.Contains(normalized, "/.tmp/") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		file := codexJSONLFile{Path: path, ModTime: info.ModTime()}
		any = append(any, file)
		if codexJSONLSessionPath(root, path) {
			sessions = append(sessions, file)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})
	sort.Slice(any, func(i, j int) bool {
		return any[i].ModTime.After(any[j].ModTime)
	})
	if len(sessions) > 0 {
		return sessions, nil
	}
	return any, nil
}

func codexJSONLSessionPath(root string, path string) bool {
	rootBase := filepath.Base(filepath.Clean(root))
	if rootBase == "sessions" {
		return true
	}
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/sessions/")
}

func readLatestCodexRateLimits(path string) (codexJSONLRateLimits, error) {
	data, err := readFileTail(path, 2*1024*1024)
	if err != nil {
		return codexJSONLRateLimits{}, err
	}
	lines := strings.Split(string(data), "\n")
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := strings.TrimSpace(lines[idx])
		if line == "" || !strings.Contains(line, `"rate_limits"`) {
			continue
		}
		var record codexJSONLRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record.Payload.RateLimits == nil {
			continue
		}
		return *record.Payload.RateLimits, nil
	}
	return codexJSONLRateLimits{}, errors.New("codex jsonl did not include rate_limits")
}

func readLatestCodexRateLimitsFromRoot(root string) (codexJSONLRateLimits, codexJSONLFile, error) {
	files, err := codexJSONLFiles(root)
	if err != nil {
		return codexJSONLRateLimits{}, codexJSONLFile{}, err
	}
	for idx, file := range files {
		if idx >= 20 {
			break
		}
		rateLimits, err := readLatestCodexRateLimits(file.Path)
		if err == nil {
			return rateLimits, file, nil
		}
	}
	return codexJSONLRateLimits{}, codexJSONLFile{}, errors.New("recent codex jsonl files did not include rate_limits")
}

func readFileTail(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		if _, err := file.Seek(info.Size()-maxBytes, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(file)
}

func codexUsageFromRateLimits(rateLimits codexJSONLRateLimits, loc *time.Location, now time.Time, language string) Usage {
	limits := []codexStatusLimit{
		codexLimitFromJSONLWindow("5h", rateLimits.Primary, loc, now, language),
		codexLimitFromJSONLWindow("weekly", rateLimits.Secondary, loc, now, language),
	}
	plan := formatCodexPlanType(rateLimits.PlanType)
	return codexUsageFromStatus(limits, plan, language)
}

func codexLimitFromJSONLWindow(defaultLabel string, window codexJSONLLimitWindow, loc *time.Location, now time.Time, language string) codexStatusLimit {
	label := defaultLabel
	switch window.WindowMinute {
	case 300:
		label = "5h"
	case 10080:
		label = "weekly"
	case 0:
	default:
		label = fmt.Sprintf("%dm", window.WindowMinute)
	}
	left := int(math.Round(100 - window.UsedPercent))
	left = int(math.Max(0, math.Min(100, float64(left))))
	return codexStatusLimit{
		Label: label,
		Left:  left,
		Reset: formatCodexResetTime(window.ResetsAt, loc, now, config.IsChinese(language)),
	}
}

func formatCodexResetTime(unixSeconds int64, loc *time.Location, now time.Time, zh bool) string {
	if unixSeconds <= 0 {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	reset := time.Unix(unixSeconds, 0).In(loc)
	now = now.In(loc)
	if reset.Year() == now.Year() && reset.YearDay() == now.YearDay() {
		return reset.Format("15:04")
	}
	if zh {
		return fmt.Sprintf("%d月%d日 %s", int(reset.Month()), reset.Day(), reset.Format("15:04"))
	}
	return reset.Format("15:04 on 2 Jan")
}

func formatCodexPlanType(plan string) string {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return ""
	}
	if len(plan) == 1 {
		return strings.ToUpper(plan)
	}
	return strings.ToUpper(plan[:1]) + strings.ToLower(plan[1:])
}

func parseCodexStatus(raw string, language string) (Usage, error) {
	text := stripTerminalControl(raw)
	lines := strings.Split(text, "\n")
	var limits []codexStatusLimit
	seen := map[string]bool{}
	plan := ""
	var lastLimit *codexStatusLimit

	for _, rawLine := range lines {
		line := normalizeStatusLine(rawLine)
		if line == "" {
			continue
		}
		if plan == "" {
			plan = parseCodexPlan(line)
		}
		if len(limits) > 0 && strings.Contains(line, "GPT-") && strings.Contains(strings.ToLower(line), "limit:") {
			break
		}
		if limit, ok := parseCodexLimitLine(line); ok {
			key := strings.ToLower(limit.Label)
			if !seen[key] {
				limits = append(limits, limit)
				seen[key] = true
				lastLimit = &limits[len(limits)-1]
			}
			continue
		}
		if lastLimit != nil && lastLimit.Reset == "" {
			if reset := parseCodexReset(line); reset != "" {
				lastLimit.Reset = reset
			}
		}
	}

	if len(limits) == 0 {
		return Usage{}, errors.New("codex /status output did not include limits")
	}
	return codexUsageFromStatus(limits, plan, language), nil
}

func codexUsageFromStatus(limits []codexStatusLimit, plan string, language string) Usage {
	remaining := 100
	for _, limit := range limits {
		if limit.Left < remaining {
			remaining = limit.Left
		}
	}
	remaining = int(math.Max(0, math.Min(100, float64(remaining))))

	zh := config.IsChinese(language)
	primary := fmt.Sprintf("%d%% left", remaining)
	if zh {
		primary = fmt.Sprintf("剩余 %d%%", remaining)
	}

	parts := make([]string, 0, len(limits)+1)
	windows := make([]UsageWindow, 0, len(limits))
	for _, limit := range limits {
		label := statusLimitLabel(limit.Label, zh)
		windowPrimary := fmt.Sprintf("%d%% left", limit.Left)
		if zh {
			windowPrimary = fmt.Sprintf("剩余 %d%%", limit.Left)
		}
		windows = append(windows, UsageWindow{
			Label:   label,
			Primary: windowPrimary,
			Reset:   limit.Reset,
			Percent: limit.Left,
		})
		if limit.Reset != "" {
			if zh {
				parts = append(parts, fmt.Sprintf("%s %d%% 重置%s", label, limit.Left, limit.Reset))
			} else {
				parts = append(parts, fmt.Sprintf("%s %d%% reset %s", label, limit.Left, limit.Reset))
			}
		} else {
			parts = append(parts, fmt.Sprintf("%s %d%%", label, limit.Left))
		}
	}
	if plan != "" {
		parts = append(parts, plan)
	}

	return Usage{Name: "Codex", Primary: primary, Secondary: strings.Join(parts, " · "), Percent: remaining, Windows: windows}
}

func parseCodexLimitLine(line string) (codexStatusLimit, bool) {
	re := regexp.MustCompile(`(?i)\b(5h|weekly)\s+limit:\s+.*?([0-9]{1,3})%\s+left(?:.*?\bresets\s+([^()│]+))?`)
	matches := re.FindStringSubmatch(line)
	if len(matches) == 0 {
		return codexStatusLimit{}, false
	}
	left, err := strconv.Atoi(matches[2])
	if err != nil {
		return codexStatusLimit{}, false
	}
	limit := codexStatusLimit{
		Label: strings.ToLower(matches[1]),
		Left:  int(math.Max(0, math.Min(100, float64(left)))),
	}
	if len(matches) > 3 {
		limit.Reset = cleanReset(matches[3])
	}
	return limit, true
}

func parseCodexReset(line string) string {
	re := regexp.MustCompile(`(?i)\bresets\s+([^()│]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) < 2 {
		return ""
	}
	return cleanReset(matches[1])
}

func parseCodexPlan(line string) string {
	re := regexp.MustCompile(`(?i)\bAccount:\s+.*\(([^)]+)\)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func statusLimitLabel(label string, zh bool) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "5h":
		if zh {
			return "5小时"
		}
		return "5h"
	case "weekly":
		if zh {
			return "每周"
		}
		return "weekly"
	default:
		return strings.TrimSpace(label)
	}
}

func cleanReset(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "│ ")
	return strings.Join(strings.Fields(value), " ")
}

func codexErrorUsage(language string) Usage {
	if config.IsChinese(language) {
		return Usage{Name: "Codex", Primary: "查询失败", Secondary: "codex jsonl 未返回用量", Percent: 0}
	}
	return Usage{Name: "Codex", Primary: "query failed", Secondary: "codex jsonl returned no usage", Percent: 0}
}

func normalizeStatusLine(line string) string {
	line = strings.ReplaceAll(line, "\r", "")
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "│╭╮╰╯─ ")
	return strings.Join(strings.Fields(line), " ")
}

func stripTerminalControl(value string) string {
	value = regexp.MustCompile(`\x1b\][^\a]*(\a|\x1b\\)`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`\x1b[@-Z\\-_]`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`).ReplaceAllString(value, "")
	return value
}

func shellCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`).MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func shortCLIOutput(value string) string {
	value = strings.TrimSpace(stripTerminalControl(value))
	if value == "" {
		return "no stderr"
	}
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 180 {
		return value[:177] + "..."
	}
	return value
}
