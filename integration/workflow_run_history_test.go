package integration

import (
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/wanglongan587/cloud/internal/core"
)

// seededRun is one run row written straight into the store: the id the read must return and the
// instant the walk must order it by.
type seededRun struct {
	id        string
	createdAt time.Time
}

// seedRun writes a terminal run row at a chosen instant. The run list is a read path, so the
// acceptance below goes through real HTTP; only the history itself is arranged directly, because a
// hundred real executions would add runtime without adding evidence about the walk.
func seedRun(t *testing.T, f *fixture, wid, snapID string, run seededRun, name string) {
	t.Helper()
	_, err := f.store.Pool.ExecContext(t.Context(),
		`INSERT INTO workflow_runs(id,tenant_id,workflow_id,snapshot_id,name,status,created_at,updated_at,started_at,finished_at)
		 VALUES($1,$2,$3,$4,$5,'succeeded',$6,$6,$6,$6)`,
		run.id, f.tid, wid, snapID, name, run.createdAt)
	must(t, err)
}

// TestWorkflowRunHistoryWalksNewestFirst pins the run list's order and its cursor.
//
// The list is a paged history, so two things have to hold at once: the newest run comes first, and
// paging reaches every run exactly once. Row ids are random UUIDs, which is why ordering by id
// (what the list used to do) cannot satisfy either — this seeds ids that are deliberately unrelated
// to time, with several runs sharing one instant so the (created_at, id) tiebreaker is exercised.
func TestWorkflowRunHistoryWalksNewestFirst(t *testing.T) {
	f := setup(t)
	seedPersonalModel(t, f)
	wf := f.call("POST", f.path("/workflows"), core.Object{"name": "History", "graph": runGraph()}, "hist-wf", 200).O("resource")
	wfid := wf.S("id")
	snapID := f.call("POST", f.path("/workflows/"+wfid+"/publish"), core.Object{}, "hist-pub", 200).O("resource").S("id")

	const total = 120
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	seeded := make([]seededRun, 0, total)
	for i := range total {
		// Two runs per instant: created_at alone is not a total order, so the id must break the tie.
		run := seededRun{id: uuid.NewString(), createdAt: base.Add(time.Duration(i/2) * time.Minute)}
		seedRun(t, f, wfid, snapID, run, "run "+run.id[:8])
		seeded = append(seeded, run)
	}
	// The order the read must produce, computed independently of the endpoint.
	want := slices.Clone(seeded)
	sort.Slice(want, func(i, j int) bool {
		if !want[i].createdAt.Equal(want[j].createdAt) {
			return want[i].createdAt.After(want[j].createdAt)
		}
		return want[i].id > want[j].id
	})

	var got []string
	cursor := ""
	for pages := 0; ; pages++ {
		require(t, pages < total, "the walk did not terminate")
		query := "?limit=7"
		if cursor != "" {
			query += "&after=" + cursor
		}
		page := f.call("GET", f.path("/workflows/"+wfid+"/runs")+query, nil, "", 200)
		items := issueItems(page)
		require(t, len(items) > 0, "a page in the middle of the walk must not be empty")
		for _, item := range items {
			got = append(got, item.S("id"))
		}
		if page.S("nextCursor") == "" {
			break
		}
		// The cursor is the page's own last row, so the next request continues strictly below it.
		require(t, page.S("nextCursor") == items[len(items)-1].S("id"), "the cursor must be the last row of the page")
		cursor = page.S("nextCursor")
	}

	gotIDs := make([]string, 0, len(want))
	for _, run := range want {
		gotIDs = append(gotIDs, run.id)
	}
	if !slices.Equal(got, gotIDs) {
		t.Fatalf("the walk returned %d rows in the wrong order or with repeats; want %d newest first", len(got), total)
	}

	// The newest run is on the first page, which is the whole point of ordering server-side: the
	// page the member sees first has to be the one they opened the history for.
	newest := f.call("GET", f.path("/workflows/"+wfid+"/runs")+"?limit=1", nil, "", 200)
	if first := issueItems(newest)[0].S("id"); first != want[0].id {
		t.Fatalf("the first page starts at %s; want the newest run %s", first, want[0].id)
	}

	// A cursor that resolves to no row of this workflow ends the walk with an empty page instead of
	// repeating the history from the top.
	stranger := f.call("GET", f.path("/workflows/"+wfid+"/runs")+"?after="+uuid.NewString(), nil, "", 200)
	if len(issueItems(stranger)) != 0 || stranger.S("nextCursor") != "" {
		t.Fatalf("a cursor outside the workflow must end the walk, got %v", stranger)
	}
	// A cursor that is not a row id at all is refused, not silently treated as the start.
	f.call("GET", f.path("/workflows/"+wfid+"/runs")+"?after=not-a-cursor", nil, "", 400)
}

// TestWorkflowRowsCarryTheirNewestRun proves a workflow read labels when it last ran.
//
// The label answers "did this workflow work last time" from the list, so it has to be the same run
// the history would show first — not one run on the list and another on the detail page.
func TestWorkflowRowsCarryTheirNewestRun(t *testing.T) {
	f := setup(t)
	seedPersonalModel(t, f)
	never := f.call("POST", f.path("/workflows"), core.Object{"name": "Never ran", "graph": runGraph()}, "lbl-wf-0", 200).O("resource")
	wf := f.call("POST", f.path("/workflows"), core.Object{"name": "Ran twice", "graph": runGraph()}, "lbl-wf", 200).O("resource")
	wfid := wf.S("id")
	snapID := f.call("POST", f.path("/workflows/"+wfid+"/publish"), core.Object{}, "lbl-pub", 200).O("resource").S("id")

	older := seededRun{id: uuid.NewString(), createdAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)}
	newer := seededRun{id: uuid.NewString(), createdAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)}
	// Written newer first so a "first row wins" implementation would answer with the older one.
	seedRun(t, f, wfid, snapID, older, "older run")
	seedRun(t, f, wfid, snapID, newer, "newer run")

	rows := issueItems(f.call("GET", f.path("/workflows")+"?limit=100", nil, "", 200))
	byName := map[string]core.Object{}
	for _, row := range rows {
		byName[row.S("name")] = row
	}
	ran, ok := byName["Ran twice"]
	require(t, ok, "the list must contain the workflow that ran")
	latest, ok := ran["latestRun"].(map[string]any)
	require(t, ok, "a workflow that ran must carry its newest run")
	if got := core.Object(latest); got.S("id") != newer.id || got.S("status") != "succeeded" || got.S("snapshotId") != snapID {
		t.Fatalf("latestRun = %v; want the newer run %s", got, newer.id)
	}
	// The detail read agrees with the list row: two surfaces must not disagree about the same run.
	detail := f.call("GET", f.path("/workflows/"+wfid), nil, "", 200)
	detailLatest, ok := detail["latestRun"].(map[string]any)
	require(t, ok, "the detail read must carry the same label")
	if got := core.Object(detailLatest); got.S("id") != newer.id {
		t.Fatalf("detail latestRun = %v; want %s", got, newer.id)
	}
	// Never having run is null rather than an absent field or a made-up run.
	if _, ok := never["latestRun"]; !ok {
		t.Fatal("a workflow that never ran must still carry latestRun, as null")
	}
	if never["latestRun"] != nil {
		t.Fatalf("a workflow that never ran reports %v; want null", never["latestRun"])
	}
	// The label is part of every read of the resource, including the create response.
	if created := byName["Never ran"]; created["latestRun"] != nil {
		t.Fatalf("a freshly created workflow reports %v; want null", created["latestRun"])
	}
}

// require fails the test with msg unless ok, keeping the assertions above readable.
func require(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal(msg)
	}
}
