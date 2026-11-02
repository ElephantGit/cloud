package store_test

import (
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestPendingMentions(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	owner := f.login(t, "owner", "owner")
	alice := f.login(t, "alice", "member")
	addProject(t, f, owner)
	task := createTask(t, f, owner, store.TaskInput{Title: "修复登录页"})
	other := createTask(t, f, owner, store.TaskInput{Title: "写周报"})
	comment := func(taskID, author, key string, mentioned ...string) {
		t.Helper()
		in := store.NewTaskComment{TenantID: f.tenant, TaskID: taskID, AuthorID: author, CreateKey: key, Body: "看一下"}
		for _, id := range mentioned {
			in.Mentions = append(in.Mentions, store.TaskMention{Type: "user", ID: id, DisplayName: "x"})
		}
		if _, e := f.store.CreateTaskComment(ctx, in); e != nil {
			t.Fatal(e)
		}
	}
	pending := func(user string) []store.PendingMention {
		t.Helper()
		got, e := f.store.PendingMentions(ctx, f.tenant, user)
		if e != nil {
			t.Fatal(e)
		}
		return got
	}

	// Mentioning oneself waits on nobody; mentioning alice waits on her, once per task, newest first.
	comment(task.ID, owner, "c1", owner)
	if got := pending(owner); len(got) != 0 {
		t.Fatalf("self mention = %+v", got)
	}
	comment(task.ID, owner, "c2", alice)
	comment(task.ID, owner, "c3", alice)
	comment(other.ID, owner, "c4", alice)
	got := pending(alice)
	if len(got) != 2 {
		t.Fatalf("pending = %+v", got)
	}
	for _, m := range got {
		if m.AuthorID != owner || m.AuthorName != "owner" {
			t.Fatalf("mention = %+v", m)
		}
	}

	// Her reply answers the task's mentions; a later mention waits again.
	comment(task.ID, alice, "r1")
	if got := pending(alice); len(got) != 1 || got[0].TaskID != other.ID {
		t.Fatalf("after reply = %+v", got)
	}
	comment(task.ID, owner, "c5", alice)
	if got := pending(alice); len(got) != 2 {
		t.Fatalf("after a new mention = %+v", got)
	}

	// A finished task waits on nobody.
	if _, e := f.store.SetTaskStatus(ctx, f.tenant, owner, other.ID, store.TaskCancelled, other.Version); e != nil {
		t.Fatal(e)
	}
	if got := pending(alice); len(got) != 1 || got[0].TaskID != task.ID {
		t.Fatalf("after cancel = %+v", got)
	}
	if _, e := f.store.PendingMentions(ctx, f.tenant, "00000000-0000-4000-8000-0000000000ff"); e == nil {
		t.Fatal("a stranger read mentions")
	}
}
