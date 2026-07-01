package dashboard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"ink-dashboard/server/internal/config"
)

type icsEvent struct {
	summary   string
	start     time.Time
	end       time.Time
	allDay    bool
	rrule     map[string]string
	exdates   []time.Time
	cancelled bool
}

type occurrence struct {
	title  string
	start  time.Time
	end    time.Time
	allDay bool
}

func (p *RealProvider) FetchCalendar(ctx context.Context, now time.Time) ([]Event, error) {
	windowStart := now.Add(-15 * time.Minute)
	windowEnd := startOfDay(now, p.location).AddDate(0, 0, p.cfg.CalendarLookaheadDay)

	var allOccurrences []occurrence
	var failures []string
	for _, calendarURL := range p.cfg.CalendarICSURLs {
		events, err := p.fetchICSCached(ctx, calendarURL)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		for _, event := range events {
			allOccurrences = append(allOccurrences, expandEvent(event, windowStart, windowEnd, p.location)...)
		}
	}

	if len(allOccurrences) == 0 && len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "; "))
	}

	sort.Slice(allOccurrences, func(i, j int) bool {
		return allOccurrences[i].start.Before(allOccurrences[j].start)
	})
	allOccurrences = dedupeOccurrences(allOccurrences, p.location)

	maxEvents := p.cfg.CalendarMaxEvents
	if maxEvents <= 0 {
		maxEvents = 4
	}
	if len(allOccurrences) > maxEvents {
		allOccurrences = allOccurrences[:maxEvents]
	}

	events := make([]Event, 0, len(allOccurrences))
	for _, occ := range allOccurrences {
		events = append(events, occurrenceToEvent(occ, now, p.location, p.cfg.Language))
	}
	return events, nil
}

func dedupeOccurrences(values []occurrence, loc *time.Location) []occurrence {
	if len(values) < 2 {
		return values
	}

	seen := make(map[string]bool, len(values))
	out := make([]occurrence, 0, len(values))
	for _, value := range values {
		key := occurrenceKey(value, loc)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func occurrenceKey(value occurrence, loc *time.Location) string {
	start := value.start.In(loc).Truncate(time.Minute)
	end := value.end.In(loc).Truncate(time.Minute)
	title := strings.Join(strings.Fields(strings.ToLower(value.title)), " ")
	return strings.Join([]string{
		title,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		strconv.FormatBool(value.allDay),
	}, "\x00")
}

func (p *RealProvider) fetchICSCached(ctx context.Context, calendarURL string) ([]icsEvent, error) {
	ttl := time.Duration(p.cfg.CalendarCacheSeconds) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	now := time.Now()
	p.cacheMu.Lock()
	cached, ok := p.icsCache[calendarURL]
	if ok && now.Sub(cached.fetchedAt) < ttl {
		events := append([]icsEvent(nil), cached.events...)
		err := cached.err
		p.cacheMu.Unlock()
		return events, err
	}
	p.cacheMu.Unlock()

	events, err := p.fetchICS(ctx, calendarURL)

	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if err != nil {
		if ok && len(cached.events) > 0 {
			return append([]icsEvent(nil), cached.events...), nil
		}
		p.icsCache[calendarURL] = icsCacheEntry{
			events:    nil,
			fetchedAt: now,
			err:       err,
		}
		return nil, err
	}
	p.icsCache[calendarURL] = icsCacheEntry{
		events:    append([]icsEvent(nil), events...),
		fetchedAt: now,
		err:       nil,
	}
	return events, nil
}

func (p *RealProvider) fetchICS(ctx context.Context, calendarURL string) ([]icsEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, calendarURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ink-dashboard/0.1")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar HTTP %d", resp.StatusCode)
	}

	maxBytes := p.cfg.CalendarMaxBytes
	if maxBytes <= 0 {
		maxBytes = 25 * 1024 * 1024
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("calendar feed exceeds %d bytes", maxBytes)
	}
	return parseICS(string(body), p.location)
}

func parseICS(content string, loc *time.Location) ([]icsEvent, error) {
	lines := unfoldICSLines(content)
	events := make([]icsEvent, 0)
	var current *icsEvent

	for _, line := range lines {
		name, params, value, ok := parseICSLine(line)
		if !ok {
			continue
		}

		switch name {
		case "BEGIN":
			if strings.EqualFold(value, "VEVENT") {
				current = &icsEvent{}
			}
		case "END":
			if strings.EqualFold(value, "VEVENT") && current != nil {
				if !current.start.IsZero() && !current.cancelled {
					if current.end.IsZero() {
						if current.allDay {
							current.end = current.start.AddDate(0, 0, 1)
						} else {
							current.end = current.start.Add(time.Hour)
						}
					}
					events = append(events, *current)
				}
				current = nil
			}
		default:
			if current == nil {
				continue
			}
			switch name {
			case "SUMMARY":
				current.summary = decodeICSText(value)
			case "DTSTART":
				start, allDay, err := parseICSTime(value, params, loc)
				if err == nil {
					current.start = start
					current.allDay = allDay
				}
			case "DTEND":
				end, _, err := parseICSTime(value, params, loc)
				if err == nil {
					current.end = end
				}
			case "RRULE":
				current.rrule = parseRRule(value)
			case "EXDATE":
				current.exdates = append(current.exdates, parseEXDate(value, params, loc)...)
			case "STATUS":
				current.cancelled = strings.EqualFold(value, "CANCELLED")
			}
		}
	}

	return events, nil
}

func unfoldICSLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += strings.TrimLeft(line, " \t")
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func parseICSLine(line string) (string, map[string]string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", nil, "", false
	}
	left := line[:idx]
	value := line[idx+1:]
	parts := strings.Split(left, ";")
	name := strings.ToUpper(parts[0])
	params := map[string]string{}
	for _, part := range parts[1:] {
		if eq := strings.Index(part, "="); eq > 0 {
			params[strings.ToUpper(part[:eq])] = strings.Trim(part[eq+1:], `"`)
		}
	}
	return name, params, value, true
}

func parseICSTime(value string, params map[string]string, fallback *time.Location) (time.Time, bool, error) {
	if params["VALUE"] == "DATE" || (!strings.Contains(value, "T") && len(value) == 8) {
		t, err := time.ParseInLocation("20060102", value, fallback)
		return t, true, err
	}

	loc := fallback
	if tzid := params["TZID"]; tzid != "" {
		if loaded, err := time.LoadLocation(tzid); err == nil {
			loc = loaded
		}
	}

	layouts := []string{"20060102T150405", "20060102T1504"}
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		if err != nil {
			return time.Time{}, false, err
		}
		return t.In(fallback), false, nil
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.In(fallback), false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("unsupported ICS time %q", value)
}

func parseEXDate(value string, params map[string]string, loc *time.Location) []time.Time {
	var times []time.Time
	for _, part := range strings.Split(value, ",") {
		t, _, err := parseICSTime(strings.TrimSpace(part), params, loc)
		if err == nil {
			times = append(times, t)
		}
	}
	return times
}

func parseRRule(value string) map[string]string {
	rule := map[string]string{}
	for _, part := range strings.Split(value, ";") {
		if eq := strings.Index(part, "="); eq > 0 {
			rule[strings.ToUpper(part[:eq])] = strings.ToUpper(part[eq+1:])
		}
	}
	return rule
}

func expandEvent(event icsEvent, from, to time.Time, loc *time.Location) []occurrence {
	if len(event.rrule) == 0 {
		return includeOccurrence(event.summary, event.start, event.end, event.allDay, from, to)
	}

	freq := event.rrule["FREQ"]
	if freq == "" {
		return nil
	}
	interval := atoiDefault(event.rrule["INTERVAL"], 1)
	countLimit := atoiDefault(event.rrule["COUNT"], 0)
	until := parseRRuleUntil(event.rrule["UNTIL"], loc)
	byDay := parseByDay(event.rrule["BYDAY"])

	duration := event.end.Sub(event.start)
	if duration <= 0 {
		duration = time.Hour
		if event.allDay {
			duration = 24 * time.Hour
		}
	}

	var out []occurrence
	generated := 0
	startDate := startOfDay(event.start, loc)
	for cursor := startDate; cursor.Before(to); cursor = cursor.AddDate(0, 0, 1) {
		if cursor.Before(startDate) {
			continue
		}
		if !matchesRRuleDate(freq, interval, byDay, event.start, cursor, loc) {
			continue
		}

		occStart := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), event.start.Hour(), event.start.Minute(), event.start.Second(), 0, loc)
		if occStart.Before(event.start) {
			continue
		}
		if !until.IsZero() && occStart.After(until) {
			break
		}

		generated++
		if countLimit > 0 && generated > countLimit {
			break
		}
		if isExcluded(occStart, event.exdates, event.allDay, loc) {
			continue
		}

		out = append(out, includeOccurrence(event.summary, occStart, occStart.Add(duration), event.allDay, from, to)...)
		if generated > 20000 {
			break
		}
	}
	return out
}

func includeOccurrence(title string, start, end time.Time, allDay bool, from, to time.Time) []occurrence {
	if title == "" {
		title = "(busy)"
	}
	if end.After(from) && start.Before(to) {
		return []occurrence{{title: title, start: start, end: end, allDay: allDay}}
	}
	return nil
}

func matchesRRuleDate(freq string, interval int, byDay map[time.Weekday]bool, eventStart, cursor time.Time, loc *time.Location) bool {
	switch freq {
	case "DAILY":
		return daysBetween(startOfDay(eventStart, loc), cursor)%interval == 0
	case "WEEKLY":
		if len(byDay) == 0 {
			byDay[eventStart.Weekday()] = true
		}
		return byDay[cursor.Weekday()] && weeksBetween(startOfWeek(eventStart, loc), startOfWeek(cursor, loc))%interval == 0
	case "MONTHLY":
		months := (cursor.Year()-eventStart.Year())*12 + int(cursor.Month()-eventStart.Month())
		return months >= 0 && months%interval == 0 && cursor.Day() == eventStart.Day()
	case "YEARLY":
		years := cursor.Year() - eventStart.Year()
		return years >= 0 && years%interval == 0 && cursor.Month() == eventStart.Month() && cursor.Day() == eventStart.Day()
	default:
		return false
	}
}

func occurrenceToEvent(occ occurrence, now time.Time, loc *time.Location, language string) Event {
	start := occ.start.In(loc)
	end := occ.end.In(loc)
	title := occ.title
	day := eventDayLabel(start, now, language)

	if occ.allDay {
		if config.IsChinese(language) {
			return Event{Day: day, Time: "全天", Title: title, Meta: "全天"}
		}
		return Event{Day: day, Time: "All day", Title: title, Meta: "all day"}
	}

	when := start.Format("15:04")
	meta := formatDuration(end.Sub(start), language)
	return Event{Day: day, Time: when, Title: title, Meta: meta}
}

func eventDayLabel(start, now time.Time, language string) string {
	if sameDay(start, now) {
		if config.IsChinese(language) {
			return "今天"
		}
		return "Today"
	}
	if config.IsChinese(language) {
		return fmt.Sprintf("%d.%d %s", int(start.Month()), start.Day(), zhWeekday(start.Weekday()))
	}
	return start.Format("Mon 1.2")
}

func zhWeekday(day time.Weekday) string {
	names := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return names[int(day)]
}

func parseRRuleUntil(value string, loc *time.Location) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _, err := parseICSTime(value, map[string]string{}, loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseByDay(value string) map[time.Weekday]bool {
	out := map[time.Weekday]bool{}
	for _, part := range strings.Split(value, ",") {
		day := strings.TrimLeft(part, "+-0123456789")
		switch day {
		case "SU":
			out[time.Sunday] = true
		case "MO":
			out[time.Monday] = true
		case "TU":
			out[time.Tuesday] = true
		case "WE":
			out[time.Wednesday] = true
		case "TH":
			out[time.Thursday] = true
		case "FR":
			out[time.Friday] = true
		case "SA":
			out[time.Saturday] = true
		}
	}
	return out
}

func isExcluded(start time.Time, exdates []time.Time, allDay bool, loc *time.Location) bool {
	for _, exdate := range exdates {
		if allDay {
			if sameDay(start.In(loc), exdate.In(loc)) {
				return true
			}
			continue
		}
		if start.Equal(exdate) {
			return true
		}
	}
	return false
}

func decodeICSText(value string) string {
	replacer := strings.NewReplacer(`\n`, " ", `\N`, " ", `\,`, ",", `\;`, ";", `\\`, `\`)
	return replacer.Replace(value)
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func startOfWeek(t time.Time, loc *time.Location) time.Time {
	day := startOfDay(t, loc)
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func daysBetween(a, b time.Time) int {
	return int(startOfDay(b, b.Location()).Sub(startOfDay(a, a.Location())).Hours() / 24)
}

func weeksBetween(a, b time.Time) int {
	return daysBetween(a, b) / 7
}

func atoiDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func formatDuration(duration time.Duration, language string) string {
	if duration <= 0 {
		return ""
	}
	minutes := int(duration.Minutes() + 0.5)
	if config.IsChinese(language) {
		if minutes < 60 {
			return fmt.Sprintf("%d 分钟", minutes)
		}
		hours := minutes / 60
		remainder := minutes % 60
		if remainder == 0 {
			return fmt.Sprintf("%d 小时", hours)
		}
		return fmt.Sprintf("%d 小时 %d 分钟", hours, remainder)
	}
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	hours := minutes / 60
	remainder := minutes % 60
	if remainder == 0 {
		return fmt.Sprintf("%d h", hours)
	}
	return fmt.Sprintf("%d h %d min", hours, remainder)
}
