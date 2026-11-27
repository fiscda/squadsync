package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxIdopontLines is how many Idopont lines a Discord modal takes.
const MaxIdopontLines = 5

// The formats of an Idopont line. The year is never typed (see
// ParseIdopontok); a day is month.day, with an optional trailing dot.
const (
	dayPattern  = `(\d{1,2})\.(\d{1,2})\.?`
	timePattern = `(\d{1,2}):(\d{2})`
)

var (
	// 12.05 20:00
	startOnlyLine = regexp.MustCompile(`^` + dayPattern + `\s+` + timePattern + `$`)
	// 12.05 20:00-23:00
	sameDayLine = regexp.MustCompile(`^` + dayPattern + `\s+` + timePattern + `\s*-\s*` + timePattern + `$`)
	// 12.05-12.07
	wholeDayLine = regexp.MustCompile(`^` + dayPattern + `\s*-\s*` + dayPattern + `$`)
	// 12.05 18:00-12.07 14:00
	rangeLine = regexp.MustCompile(`^` + dayPattern + `\s+` + timePattern + `\s*-\s*` + dayPattern + `\s+` + timePattern + `$`)
)

// Reasons a line is refused.
const (
	reasonFormat = "érvénytelen formátum (például 12.05 20:00, 12.05 20:00-23:00, 12.05-12.07 vagy 12.05 18:00-12.07 14:00)"
	reasonDate   = "érvénytelen dátum"
	reasonTime   = "érvénytelen óra:perc"
	reasonEnd    = "a vége nem lehet a kezdete előtt vagy vele egyidőben"
)

// IdopontParseError says, in Hungarian and ready to show to the user, why
// the Idopont lines were refused. It wraps ErrInvalidInput.
type IdopontParseError struct {
	// Line is the 1-based number of the offending line, as the user typed
	// them (blank lines count); 0 when the text as a whole is at fault.
	Line int
	Text string
	// Reason is Hungarian.
	Reason string
}

func (e *IdopontParseError) Error() string {
	if e.Line == 0 {
		return e.Reason
	}
	return fmt.Sprintf("%d. sor („%s”): %s", e.Line, e.Text, e.Reason)
}

func (e *IdopontParseError) Unwrap() error { return ErrInvalidInput }

// ParseIdopontok reads the Idopont lines typed into Discord's /esemeny
// modal, one Idopont per line, in exactly these formats (month.day):
//
//	12.05 20:00             start only
//	12.05 20:00-23:00       end on the same day
//	12.05-12.07             whole days, both included (EgeszNapos)
//	12.05 18:00-12.07 14:00 start and end, possibly on different days
//
// Whitespace around a line, a trailing dot after a day, single-digit
// numbers and a "–" for the "-" are accepted. Blank lines are ignored.
//
// The year is not typed. A start is in the current Budapest year (by now),
// unless that day is already over, then in the next one. An end is in the
// first year, from the start's, in which it isn't before the start, so
// "12.30-01.02" crosses New Year. A same-day end must be after the start;
// it never wraps past midnight.
//
// At most MaxIdopontLines lines and at least one are allowed. A refusal is
// an *IdopontParseError.
func ParseIdopontok(text string, now time.Time) ([]IdopontInput, error) {
	today := civilOf(now.In(Budapest))
	var out []IdopontInput
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if len(out) == MaxIdopontLines {
			return nil, &IdopontParseError{Reason: fmt.Sprintf("legfeljebb %d időpontot adhatsz meg", MaxIdopontLines)}
		}
		in, reason := parseIdopontLine(strings.ReplaceAll(line, "–", "-"), today)
		if reason != "" {
			return nil, &IdopontParseError{Line: n + 1, Text: line, Reason: reason}
		}
		out = append(out, in)
	}
	if len(out) == 0 {
		return nil, &IdopontParseError{Reason: "adj meg legalább egy időpontot"}
	}
	return out, nil
}

// ParseIdopontok is the package function, with the application's clock.
func (a *App) ParseIdopontok(text string) ([]IdopontInput, error) {
	return ParseIdopontok(text, a.clock.Now())
}

// EsemenyURL is the web page of an Esemeny.
func (a *App) EsemenyURL(esemenyID string) string { return a.esemenyURL(esemenyID) }

// DiscordLinkable tells whether Discord shows u as a link (see
// discordLinkable); the bot prints a URL it can't link as plain text.
func DiscordLinkable(u string) bool { return discordLinkable(u) }

// civil is a calendar day.
type civil struct{ year, month, day int }

func civilOf(t time.Time) civil { return civil{t.Year(), int(t.Month()), t.Day()} }

func (c civil) before(o civil) bool {
	if c.year != o.year {
		return c.year < o.year
	}
	if c.month != o.month {
		return c.month < o.month
	}
	return c.day < o.day
}

func (c civil) exists() bool {
	t := time.Date(c.year, time.Month(c.month), c.day, 0, 0, 0, 0, time.UTC)
	return civilOf(t) == c
}

func (c civil) String() string { return fmt.Sprintf("%04d-%02d-%02d", c.year, c.month, c.day) }

// moment is a day and a minute of it.
type moment struct {
	day     civil
	minutes int
}

func (m moment) after(o moment) bool {
	if m.day != o.day {
		return o.day.before(m.day)
	}
	return m.minutes > o.minutes
}

func (m moment) String() string {
	return fmt.Sprintf("%sT%02d:%02d", m.day, m.minutes/60, m.minutes%60)
}

// parseIdopontLine reads one trimmed line (en dashes already replaced). It
// returns the reason it is refused, or "".
func parseIdopontLine(line string, today civil) (IdopontInput, string) {
	switch {
	case startOnlyLine.MatchString(line):
		g := numbers(startOnlyLine, line)
		start, reason := startMoment(g[0], g[1], g[2], g[3], today)
		if reason != "" {
			return IdopontInput{}, reason
		}
		return IdopontInput{Start: start.String()}, ""

	case sameDayLine.MatchString(line):
		g := numbers(sameDayLine, line)
		start, reason := startMoment(g[0], g[1], g[2], g[3], today)
		if reason != "" {
			return IdopontInput{}, reason
		}
		endMinutes, ok := minutesOf(g[4], g[5])
		if !ok {
			return IdopontInput{}, reasonTime
		}
		end := moment{start.day, endMinutes}
		if !end.after(start) {
			return IdopontInput{}, reasonEnd
		}
		return IdopontInput{Start: start.String(), End: end.String()}, ""

	case wholeDayLine.MatchString(line):
		g := numbers(wholeDayLine, line)
		first, ok := nextDay(g[0], g[1], today)
		if !ok {
			return IdopontInput{}, reasonDate
		}
		last, ok := nextDay(g[2], g[3], first)
		if !ok {
			return IdopontInput{}, reasonDate
		}
		return IdopontInput{Start: first.String(), End: last.String(), EgeszNapos: true}, ""

	case rangeLine.MatchString(line):
		g := numbers(rangeLine, line)
		start, reason := startMoment(g[0], g[1], g[2], g[3], today)
		if reason != "" {
			return IdopontInput{}, reason
		}
		endDay, ok := nextDay(g[4], g[5], start.day)
		if !ok {
			return IdopontInput{}, reasonDate
		}
		endMinutes, ok := minutesOf(g[6], g[7])
		if !ok {
			return IdopontInput{}, reasonTime
		}
		end := moment{endDay, endMinutes}
		if !end.after(start) {
			return IdopontInput{}, reasonEnd
		}
		return IdopontInput{Start: start.String(), End: end.String()}, ""
	}
	return IdopontInput{}, reasonFormat
}

// numbers returns the captured groups of a line that matched re, as ints.
func numbers(re *regexp.Regexp, line string) []int {
	groups := re.FindStringSubmatch(line)[1:]
	out := make([]int, len(groups))
	for n, g := range groups {
		out[n], _ = strconv.Atoi(g) // at most two digits, as the pattern says
	}
	return out
}

// startMoment resolves a start's day and time.
func startMoment(month, day, hour, minute int, today civil) (moment, string) {
	d, ok := nextDay(month, day, today)
	if !ok {
		return moment{}, reasonDate
	}
	minutes, ok := minutesOf(hour, minute)
	if !ok {
		return moment{}, reasonTime
	}
	return moment{d, minutes}, ""
}

func minutesOf(hour, minute int) (int, bool) {
	if hour > 23 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// nextDay is the first month.day that is on or after from. It reports false
// for a day that never exists, such as 02.30. Looking up to 8 years ahead
// finds a 02.29 whatever the year.
func nextDay(month, day int, from civil) (civil, bool) {
	for y := from.year; y <= from.year+8; y++ {
		c := civil{y, month, day}
		if c.exists() && !c.before(from) {
			return c, true
		}
	}
	return civil{}, false
}
