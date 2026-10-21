package core

import (
	"reflect"
	"testing"
	"time"
)

func TestParseScheduledTaskSchedule(t *testing.T) {
	valid := []struct {
		name string
		raw  any
		want scheduledTaskSchedule
	}{
		{"every_hours", Object{"kind": "every_hours", "intervalHours": 6}, scheduledTaskSchedule{Kind: "every_hours", IntervalHours: 6}},
		{"daily", Object{"kind": "daily", "hour": 9, "minute": 30}, scheduledTaskSchedule{Kind: "daily", Hour: 9, Minute: 30}},
		{"weekly", Object{"kind": "weekly", "hour": 18, "minute": 0, "weekdays": []string{"monday", "friday"}},
			scheduledTaskSchedule{Kind: "weekly", Hour: 18, Weekdays: []time.Weekday{time.Monday, time.Friday}}},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseScheduledTaskSchedule(tc.raw)
			if err != nil {
				t.Fatalf("parseScheduledTaskSchedule(%v) = %v", tc.raw, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseScheduledTaskSchedule(%v) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}

	for _, raw := range []any{
		Object{"kind": "cron", "expr": "* * * * *"},
		Object{"kind": "every_hours", "intervalHours": 0},
		Object{"kind": "daily", "hour": 24, "minute": 0},
		Object{"kind": "daily", "hour": 9, "minute": 60},
		Object{"kind": "weekly", "hour": 9, "minute": 0, "weekdays": []string{}},
		Object{"kind": "weekly", "hour": 9, "minute": 0, "weekdays": []string{"monday", "funday"}},
	} {
		if _, err := parseScheduledTaskSchedule(raw); err == nil {
			t.Errorf("parseScheduledTaskSchedule(%v) = nil error, want rejection", raw)
		}
	}
}

func TestScheduleJSONCanonical(t *testing.T) {
	cases := []struct {
		in   scheduledTaskSchedule
		want Object
	}{
		{scheduledTaskSchedule{Kind: "every_hours", IntervalHours: 3}, Object{"kind": "every_hours", "intervalHours": 3}},
		{scheduledTaskSchedule{Kind: "daily", Hour: 9, Minute: 30}, Object{"kind": "daily", "hour": 9, "minute": 30}},
		{scheduledTaskSchedule{Kind: "weekly", Hour: 18, Weekdays: []time.Weekday{time.Monday, time.Friday}},
			Object{"kind": "weekly", "weekdays": []string{"monday", "friday"}, "hour": 18, "minute": 0}},
	}
	for _, tc := range cases {
		got, err := scheduleJSON(tc.in)
		if err != nil {
			t.Fatalf("scheduleJSON(%+v) = %v", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("scheduleJSON(%+v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNextDueWallClock(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	day := func(y int, m time.Month, d, hh, mm int) time.Time {
		return time.Date(y, m, d, hh, mm, 0, 0, loc)
	}
	daily := scheduledTaskSchedule{Kind: "daily", Hour: 9, Minute: 0}
	if got := daily.nextDue(day(2026, 1, 1, 8, 0), loc); !got.Equal(day(2026, 1, 1, 9, 0)) {
		t.Fatalf("daily.nextDue before the wall clock = %v", got)
	}
	if got := daily.nextDue(day(2026, 1, 1, 9, 0), loc); !got.Equal(day(2026, 1, 2, 9, 0)) {
		t.Fatalf("daily.nextDue exactly at the wall clock = %v", got)
	}

	weekly := scheduledTaskSchedule{Kind: "weekly", Hour: 18, Minute: 0, Weekdays: []time.Weekday{time.Monday, time.Friday}}
	// 2026-01-01 is a Thursday; the next Friday is 2026-01-02.
	if got := weekly.nextDue(day(2026, 1, 1, 12, 0), loc); !got.Equal(day(2026, 1, 2, 18, 0)) {
		t.Fatalf("weekly.nextDue from Thursday = %v", got)
	}
	// From Friday 20:00 (after the wall clock), the next due is Monday.
	if got := weekly.nextDue(day(2026, 1, 2, 20, 0), loc); !got.Equal(day(2026, 1, 5, 18, 0)) {
		t.Fatalf("weekly.nextDue past Friday = %v", got)
	}

	hourly := scheduledTaskSchedule{Kind: "every_hours", IntervalHours: 6}
	if got := hourly.nextDue(day(2026, 1, 1, 0, 0), loc); !got.Equal(day(2026, 1, 1, 6, 0)) {
		t.Fatalf("every_hours.nextDue = %v", got)
	}
}

func TestNextDueSpringForwardNeverRetries(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-03-08 exists at 02:34 local; 02:35 does not (spring forward). The 02:34 daily slot is
	// normalized forward and is not re-emitted, matching "DST 跳过不补跑".
	daily := scheduledTaskSchedule{Kind: "daily", Hour: 2, Minute: 34}
	after := time.Date(2026, 3, 8, 2, 0, 0, 0, loc)
	got := daily.nextDue(after, loc)
	if got.IsDST() || got.Sub(after) <= 0 {
		t.Fatalf("spring-forward 02:34 must normalize to a later non-DST instant, got %v", got)
	}
}

func TestDuePointRespectsResumeAndAnchor(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	daily := scheduledTaskSchedule{Kind: "daily", Hour: 9, Minute: 0}
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	resume := time.Date(2026, 1, 3, 9, 0, 0, 0, loc)
	if got := duePoint(daily, &resume, anchor, loc); !got.Equal(time.Date(2026, 1, 4, 9, 0, 0, 0, loc)) {
		t.Fatalf("duePoint past resume = %v", got)
	}
	if got := duePoint(daily, nil, anchor, loc); !got.Equal(time.Date(2026, 1, 1, 9, 0, 0, 0, loc)) {
		t.Fatalf("duePoint with no resume = %v", got)
	}
}
