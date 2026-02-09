package forumbus

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"metawsm/internal/model"
	"metawsm/internal/policy"
	"metawsm/internal/store"
)

func TestRuntimePublishAndProcessOnce(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	sqliteStore := store.NewSQLiteStore(dbPath)
	if err := sqliteStore.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	rt := NewRuntime(sqliteStore, policy.Default())
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	defer rt.Stop()

	var handled int32
	if err := rt.RegisterHandler("forum.commands.open_thread", func(ctx context.Context, message model.ForumOutboxMessage) error {
		_ = ctx
		if message.Topic != "forum.commands.open_thread" {
			t.Fatalf("unexpected topic %s", message.Topic)
		}
		atomic.AddInt32(&handled, 1)
		return nil
	}); err != nil {
		t.Fatalf("register handler: %v", err)
	}

	if _, err := rt.Publish("forum.commands.open_thread", "thread-1", map[string]any{"thread_id": "thread-1"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	processed, err := rt.ProcessOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("process once: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected processed=1, got %d", processed)
	}
	if atomic.LoadInt32(&handled) != 1 {
		t.Fatalf("expected handler invocation count 1, got %d", handled)
	}
	sent, err := sqliteStore.ListForumOutboxByStatus(model.ForumOutboxStatusSent, 10)
	if err != nil {
		t.Fatalf("list sent outbox: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected one sent outbox message, got %d", len(sent))
	}
}
