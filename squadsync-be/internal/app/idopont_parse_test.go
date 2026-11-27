package app_test

import (
	"errors"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
)

// budapest builds a moment of Budapest wall clock time.
func budapest(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, app.Budapest)
}

func TestParseIdopontok(t *testing.T) {
	now := budapest(2026, time.March, 2, 10, 0)
	evening := func(start string) app.IdopontInput { return app.IdopontInput{Start: start} }

	tests := []struct {
		name string
		text string
		now  time.Time
		want []app.IdopontInput
	}{
		{name: "start only", text: "12.05 20:00", want: []app.IdopontInput{evening("2026-12-05T20:00")}},
		{name: "same-day end", text: "12.05 20:00-23:00", want: []app.IdopontInput{{Start: "2026-12-05T20:00", End: "2026-12-05T23:00"}}},
		{name: "whole days", text: "12.05-12.07", want: []app.IdopontInput{{Start: "2026-12-05", End: "2026-12-07", EgeszNapos: true}}},
		{name: "start and end on different days", text: "12.05 18:00-12.07 14:00", want: []app.IdopontInput{{Start: "2026-12-05T18:00", End: "2026-12-07T14:00"}}},
		{name: "start and end on the same day", text: "12.05 18:00-12.05 21:30", want: []app.IdopontInput{{Start: "2026-12-05T18:00", End: "2026-12-05T21:30"}}},
		{name: "single digits", text: "1.5 9:00", want: []app.IdopontInput{evening("2027-01-05T09:00")}},
		{name: "single digits in a range", text: "7.4 9:05-7.6 8:00", want: []app.IdopontInput{{Start: "2026-07-04T09:05", End: "2026-07-06T08:00"}}},
		{name: "trailing dots", text: "12.05. 20:00", want: []app.IdopontInput{evening("2026-12-05T20:00")}},
		{name: "trailing dots on whole days", text: "12.05.-12.07.", want: []app.IdopontInput{{Start: "2026-12-05", End: "2026-12-07", EgeszNapos: true}}},
		{name: "trailing dots on a range", text: "12.05. 18:00-12.07. 14:00", want: []app.IdopontInput{{Start: "2026-12-05T18:00", End: "2026-12-07T14:00"}}},
		{name: "en dash in a time range", text: "12.05 20:00–23:00", want: []app.IdopontInput{{Start: "2026-12-05T20:00", End: "2026-12-05T23:00"}}},
		{name: "en dash in whole days", text: "12.05–12.07", want: []app.IdopontInput{{Start: "2026-12-05", End: "2026-12-07", EgeszNapos: true}}},
		{name: "spaces around the dash", text: "12.05 20:00 - 23:00", want: []app.IdopontInput{{Start: "2026-12-05T20:00", End: "2026-12-05T23:00"}}},
		{name: "surrounding whitespace and CRLF", text: "  \t12.05 20:00  \r\n", want: []app.IdopontInput{evening("2026-12-05T20:00")}},
		{name: "blank lines are ignored", text: "\n12.05 20:00\n   \n\n12.06 19:00\n", want: []app.IdopontInput{evening("2026-12-05T20:00"), evening("2026-12-06T19:00")}},
		{
			name: "five lines",
			text: "12.01 20:00\n12.02 20:00\n12.03 20:00\n12.04 20:00\n12.05 20:00",
			want: []app.IdopontInput{
				evening("2026-12-01T20:00"), evening("2026-12-02T20:00"), evening("2026-12-03T20:00"),
				evening("2026-12-04T20:00"), evening("2026-12-05T20:00"),
			},
		},
		{name: "today is not past", text: "03.02 08:00", want: []app.IdopontInput{evening("2026-03-02T08:00")}},
		{name: "a past day is next year", text: "03.01 20:00", want: []app.IdopontInput{evening("2027-03-01T20:00")}},
		{name: "a past January is next year", text: "01.15 20:00", want: []app.IdopontInput{evening("2027-01-15T20:00")}},
		{name: "past whole days are next year", text: "02.10-02.12", want: []app.IdopontInput{{Start: "2027-02-10", End: "2027-02-12", EgeszNapos: true}}},
		{name: "end before the start day crosses New Year", text: "12.30-01.02", want: []app.IdopontInput{{Start: "2026-12-30", End: "2027-01-02", EgeszNapos: true}}},
		{name: "start and end cross New Year", text: "12.31 22:00-01.01 02:00", want: []app.IdopontInput{{Start: "2026-12-31T22:00", End: "2027-01-01T02:00"}}},
		{name: "past start and an end across New Year", text: "02.20 12:00-01.05 12:00", want: []app.IdopontInput{{Start: "2027-02-20T12:00", End: "2028-01-05T12:00"}}},
		{name: "one-day whole range", text: "12.05-12.05", want: []app.IdopontInput{{Start: "2026-12-05", End: "2026-12-05", EgeszNapos: true}}},
		{name: "Feb 29 waits for a leap year", text: "02.29 20:00", want: []app.IdopontInput{evening("2028-02-29T20:00")}},
		{
			name: "the year follows the Budapest date, not UTC",
			text: "01.01 12:00",
			now:  time.Date(2026, time.December, 31, 23, 30, 0, 0, time.UTC), // already 2027-01-01 in Budapest
			want: []app.IdopontInput{evening("2027-01-01T12:00")},
		},
		{
			name: "in the new year a December day is next December",
			text: "12.05 20:00",
			now:  budapest(2027, time.January, 3, 9, 0),
			want: []app.IdopontInput{evening("2027-12-05T20:00")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			at := tc.now
			if at.IsZero() {
				at = now
			}
			got, err := app.ParseIdopontok(tc.text, at)
			if err != nil {
				t.Fatalf("ParseIdopontok(%q): %v", tc.text, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseIdopontok(%q) = %+v, want %+v", tc.text, got, tc.want)
			}
			for n := range got {
				if got[n] != tc.want[n] {
					t.Errorf("idopont %d = %+v, want %+v", n+1, got[n], tc.want[n])
				}
			}
		})
	}
}

func TestParseIdopontokRefusesWithAHungarianMessageNamingTheLine(t *testing.T) {
	now := budapest(2026, time.March, 2, 10, 0)
	const formats = "érvénytelen formátum (például 12.05 20:00, 12.05 20:00-23:00, 12.05-12.07 vagy 12.05 18:00-12.07 14:00)"

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "invalid day of a month", text: "02.30 20:00", want: "1. sor („02.30 20:00”): érvénytelen dátum"},
		{name: "month 13", text: "13.01 20:00", want: "1. sor („13.01 20:00”): érvénytelen dátum"},
		{name: "month 0", text: "0.5 20:00", want: "1. sor („0.5 20:00”): érvénytelen dátum"},
		{name: "April 31", text: "04.31-05.02", want: "1. sor („04.31-05.02”): érvénytelen dátum"},
		{name: "invalid end day", text: "05.01 10:00-06.31 10:00", want: "1. sor („05.01 10:00-06.31 10:00”): érvénytelen dátum"},
		{name: "invalid end day of whole days", text: "05.01-02.30", want: "1. sor („05.01-02.30”): érvénytelen dátum"},
		{name: "the line number is the typed one, blank lines count", text: "12.05 20:00\n\n12.40 20:00", want: "3. sor („12.40 20:00”): érvénytelen dátum"},
		{name: "hour 24", text: "12.05 24:00", want: "1. sor („12.05 24:00”): érvénytelen óra:perc"},
		{name: "minute 60", text: "12.05 20:60", want: "1. sor („12.05 20:60”): érvénytelen óra:perc"},
		{name: "bad end time", text: "12.05 20:00-25:00", want: "1. sor („12.05 20:00-25:00”): érvénytelen óra:perc"},
		{name: "end before the start on the same day", text: "12.05 20:00-18:00", want: "1. sor („12.05 20:00-18:00”): a vége nem lehet a kezdete előtt vagy vele egyidőben"},
		{name: "end equal to the start", text: "12.05 20:00-20:00", want: "1. sor („12.05 20:00-20:00”): a vége nem lehet a kezdete előtt vagy vele egyidőben"},
		{name: "no wrapping past midnight", text: "12.05 22:00-02:00", want: "1. sor („12.05 22:00-02:00”): a vége nem lehet a kezdete előtt vagy vele egyidőben"},
		{name: "end before the start on the same date", text: "12.05 20:00-12.05 18:00", want: "1. sor („12.05 20:00-12.05 18:00”): a vége nem lehet a kezdete előtt vagy vele egyidőben"},
		{name: "a day alone", text: "12.05", want: "1. sor („12.05”): " + formats},
		{name: "a time alone", text: "20:00", want: "1. sor („20:00”): " + formats},
		{name: "a year is not accepted", text: "2026.12.05 20:00", want: "1. sor („2026.12.05 20:00”): " + formats},
		{name: "a text", text: "holnap este", want: "1. sor („holnap este”): " + formats},
		{name: "trailing words", text: "12.05 20:00 körül", want: "1. sor („12.05 20:00 körül”): " + formats},
		{name: "mixed whole day and time", text: "12.05-12.07 20:00", want: "1. sor („12.05-12.07 20:00”): " + formats},
		{name: "ISO date", text: "2026-12-05 20:00", want: "1. sor („2026-12-05 20:00”): " + formats},
		{name: "the first bad line is named", text: "12.05 20:00\n12.06 20:00\nxx\n13.01 20:00", want: "3. sor („xx”): " + formats},
		{name: "empty", text: "", want: "adj meg legalább egy időpontot"},
		{name: "only blank lines", text: " \n\n \t", want: "adj meg legalább egy időpontot"},
		{
			name: "more than five lines",
			text: "12.01 20:00\n12.02 20:00\n12.03 20:00\n12.04 20:00\n12.05 20:00\n12.06 20:00",
			want: "legfeljebb 5 időpontot adhatsz meg",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := app.ParseIdopontok(tc.text, now)
			if err == nil {
				t.Fatalf("ParseIdopontok(%q) = %+v, want an error", tc.text, got)
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err, tc.want)
			}
			if !errors.Is(err, app.ErrInvalidInput) {
				t.Errorf("error = %v, want it to wrap ErrInvalidInput", err)
			}
		})
	}
}

func TestParseIdopontokUsesTheAppClock(t *testing.T) {
	h := apptest.New(t) // starts on 2026-03-02
	got, err := h.App.ParseIdopontok("03.01 20:00")
	if err != nil || len(got) != 1 || got[0].Start != "2027-03-01T20:00" {
		t.Fatalf("ParseIdopontok = %+v, %v; want 2027-03-01T20:00", got, err)
	}
	h.Clock.Set(budapest(2027, time.January, 10, 12, 0))
	got, err = h.App.ParseIdopontok("03.01 20:00")
	if err != nil || len(got) != 1 || got[0].Start != "2027-03-01T20:00" {
		t.Fatalf("ParseIdopontok in 2027 = %+v, %v; want 2027-03-01T20:00", got, err)
	}
	h.Clock.Set(budapest(2027, time.March, 2, 12, 0))
	got, err = h.App.ParseIdopontok("03.01 20:00")
	if err != nil || len(got) != 1 || got[0].Start != "2028-03-01T20:00" {
		t.Fatalf("ParseIdopontok after it = %+v, %v; want 2028-03-01T20:00", got, err)
	}
}
