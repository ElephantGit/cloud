package schedule

import (
	"testing"
	"time"
)

func instant(s string) time.Time {
	v, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return v
}
func TestNext(t *testing.T) {
	tests := []struct {
		name                 string
		rule                 Rule
		created, after, want string
	}{
		{"year rollover", Rule{Kind: "daily", Time: "09:00", Timezone: "Asia/Shanghai"}, "2026-01-01T00:00:00Z", "2026-12-31T01:00:00Z", "2027-01-01T01:00:00Z"},
		{"weekend", Rule{Kind: "weekdays", Time: "09:00", Timezone: "Asia/Shanghai"}, "2026-01-01T00:00:00Z", "2026-10-30T02:00:00Z", "2026-11-02T01:00:00Z"},
		{"weekly", Rule{Kind: "weekly", Time: "09:00", Days: []int{5}, Timezone: "UTC"}, "2026-01-01T00:00:00Z", "2026-10-30T09:00:00Z", "2026-11-06T09:00:00Z"},
		{"DST gap", Rule{Kind: "daily", Time: "02:30", Timezone: "America/New_York"}, "2026-01-01T00:00:00Z", "2026-03-08T00:00:00Z", "2026-03-09T06:30:00Z"},
		{"DST fold first", Rule{Kind: "daily", Time: "01:30", Timezone: "America/New_York"}, "2026-01-01T00:00:00Z", "2026-11-01T00:00:00Z", "2026-11-01T05:30:00Z"},
		{"DST fold no second", Rule{Kind: "daily", Time: "01:30", Timezone: "America/New_York"}, "2026-01-01T00:00:00Z", "2026-11-01T05:30:00Z", "2026-11-02T06:30:00Z"},
		{"anchored interval", Rule{Kind: "hours", Hours: 6, Timezone: "Asia/Shanghai"}, "2026-10-29T01:00:00Z", "2026-10-29T08:30:00Z", "2026-10-29T13:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, e := Next(tt.rule, instant(tt.created), instant(tt.after), 3)
			if e != nil || len(got) != 3 || !got[0].Equal(instant(tt.want)) {
				t.Fatalf("%v %v", got, e)
			}
			for i := 1; i < len(got); i++ {
				if !got[i].After(got[i-1]) {
					t.Fatal("unordered times")
				}
			}
		})
	}
}
func TestInvalidRules(t *testing.T) {
	for _, r := range []Rule{{Kind: "hours", Hours: 0, Timezone: "UTC"}, {Kind: "weekly", Time: "09:00", Timezone: "UTC"}, {Kind: "daily", Time: "25:00", Timezone: "UTC"}, {Kind: "daily", Time: "09:00", Timezone: "Local"}, {Kind: "weekly", Time: "09:00", Timezone: "UTC", Days: []int{1, 1}}} {
		if _, e := Next(r, instant("2026-01-01T00:00:00Z"), instant("2026-02-01T00:00:00Z"), 1); e == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
}
