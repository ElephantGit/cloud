// Package schedule calculates configuration times without dispatching work.
package schedule

import (
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"
)

// Rule matches the saved v2 schedule configuration.
type Rule struct {
	Kind     string `json:"kind"`
	Time     string `json:"time"`
	Days     []int  `json:"days"`
	Hours    int    `json:"hours"`
	Timezone string `json:"timezone"`
}

// Next returns up to three instants strictly after after, using created as the interval anchor.
// Calendar gaps are skipped; a repeated local time fires only at its first occurrence.
func Next(r Rule, created, after time.Time, count int) ([]time.Time, error) {
	if count < 1 || count > 3 || created.IsZero() || after.IsZero() {
		return nil, errors.New("invalid preview arguments")
	}
	if r.Timezone == "" || r.Timezone == "Local" {
		return nil, errors.New("invalid timezone")
	}
	loc, e := time.LoadLocation(r.Timezone)
	if e != nil {
		return nil, e
	}
	out := []time.Time{}
	if r.Kind == "hours" {
		if r.Hours < 1 || r.Hours > 24 {
			return nil, errors.New("invalid interval")
		}
		step := time.Duration(r.Hours) * time.Hour
		n := after.Sub(created)/step + 1
		if n < 1 {
			n = 1
		}
		for i := 0; i < count; i++ {
			out = append(out, created.Add((n+time.Duration(i))*step).UTC())
		}
		return out, nil
	}
	if r.Kind != "daily" && r.Kind != "weekdays" && r.Kind != "weekly" {
		return nil, errors.New("invalid frequency")
	}
	parsed, e := time.Parse("15:04", r.Time)
	if e != nil {
		return nil, e
	}
	days := map[int]bool{}
	for _, d := range r.Days {
		if d < 0 || d > 6 || days[d] {
			return nil, errors.New("invalid weekdays")
		}
		days[d] = true
	}
	if r.Kind == "weekly" && len(days) == 0 {
		return nil, errors.New("empty weekdays")
	}
	if after.Before(created) {
		after = created
	}
	local := after.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	for d := 0; d < 32 && len(out) < count; d++ {
		date := start.AddDate(0, 0, d)
		weekday := int(date.Weekday())
		if r.Kind == "weekdays" && (weekday == 0 || weekday == 6) || r.Kind == "weekly" && !days[weekday] {
			continue
		}
		wall := date.Add(time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute)
		// Search UTC in ascending order, independent of time.Date's ambiguous-time choice.
		for candidate := wall.Add(-24 * time.Hour); !candidate.After(wall.Add(24 * time.Hour)); candidate = candidate.Add(time.Minute) {
			p := candidate.In(loc)
			if p.Year() == date.Year() && p.Month() == date.Month() && p.Day() == date.Day() && p.Hour() == parsed.Hour() && p.Minute() == parsed.Minute() {
				if candidate.After(after) {
					out = append(out, candidate)
				}
				break
			}
		}
	}
	if len(out) != count {
		return nil, fmt.Errorf("no valid times in preview horizon")
	}
	return out, nil
}
