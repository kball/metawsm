package forumbus

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"metawsm/internal/model"
	"metawsm/internal/policy"
	"metawsm/internal/store"
)

func startTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	return server
}

func testPolicyWithRedis(server *miniredis.Miniredis) policy.Config {
	cfg := policy.Default()
	cfg.Forum.Redis.URL = "redis://" + server.Addr() + "/0"
	cfg.Forum.Redis.Stream = "metawsm-forum-test"
	cfg.Forum.Redis.Group = "metawsm-forum-test"
	cfg.Forum.Redis.Consumer = "metawsm-runtime-test"
	return cfg
}

func TestRuntimePublishAndProcessOnce(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	sqliteStore := store.NewSQLiteStore(dbPath)
	if err := sqliteStore.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	redisServer := startTestRedis(t)
	rt := NewRuntime(sqliteStore, testPolicyWithRedis(redisServer))
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
	if processed < 1 {
		t.Fatalf("expected processed>=1, got %d", processed)
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

func TestRuntimeFailsWhenRedisUnavailable(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	sqliteStore := store.NewSQLiteStore(dbPath)
	if err := sqliteStore.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	cfg := policy.Default()
	cfg.Forum.Redis.URL = ""
	rt := NewRuntime(sqliteStore, cfg)
	if err := rt.Start(context.Background()); err != nil {
		// Start should fail if URL is empty.
	} else {
		defer rt.Stop()
		t.Fatalf("expected runtime start to fail when redis url is empty")
	}
	if _, err := rt.ProcessOnce(context.Background(), 10); err == nil {
		t.Fatalf("expected process once to fail when redis is unavailable")
	}

	redisServer := startTestRedis(t)
	cfg = testPolicyWithRedis(redisServer)
	rt = NewRuntime(sqliteStore, cfg)
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start runtime for mid-run outage test: %v", err)
	}
	defer rt.Stop()
	redisServer.Close()
	if _, err := rt.ProcessOnce(context.Background(), 10); err == nil {
		t.Fatalf("expected process once to fail after mid-run redis outage")
	}
}

func TestRuntimeReplaysFailedOutboxMessageAfterHandlerRegistration(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	dbPath := filepath.Join(t.TempDir(), "metawsm.db")
	sqliteStore := store.NewSQLiteStore(dbPath)
	if err := sqliteStore.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	redisServer := startTestRedis(t)
	rt := NewRuntime(sqliteStore, testPolicyWithRedis(redisServer))
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	defer rt.Stop()

	if _, err := rt.Publish("forum.commands.replay_test", "thread-replay", map[string]any{"ok": true}); err != nil {
		t.Fatalf("publish replay test message: %v", err)
	}
	if _, err := rt.ProcessOnce(context.Background(), 10); err != nil {
		t.Fatalf("process without handler: %v", err)
	}

	failed, err := sqliteStore.ListForumOutboxByStatus(model.ForumOutboxStatusFailed, 10)
	if err != nil {
		t.Fatalf("list failed outbox messages: %v", err)
	}
	if len(failed) != 1 {
		t.Fatalf("expected one failed outbox message, got %d", len(failed))
	}

	var handled int32
	if err := rt.RegisterHandler("forum.commands.replay_test", func(ctx context.Context, message model.ForumOutboxMessage) error {
		_ = ctx
		if message.Topic != "forum.commands.replay_test" {
			t.Fatalf("unexpected topic %s", message.Topic)
		}
		atomic.AddInt32(&handled, 1)
		return nil
	}); err != nil {
		t.Fatalf("register replay handler: %v", err)
	}
	if _, err := rt.ProcessOnce(context.Background(), 10); err != nil {
		t.Fatalf("process replay message: %v", err)
	}
	if atomic.LoadInt32(&handled) != 1 {
		t.Fatalf("expected replay handler invocation count 1, got %d", handled)
	}

	sent, err := sqliteStore.ListForumOutboxByStatus(model.ForumOutboxStatusSent, 10)
	if err != nil {
		t.Fatalf("list sent outbox messages: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected replayed message to move to sent, got %d sent rows", len(sent))
	}
}
