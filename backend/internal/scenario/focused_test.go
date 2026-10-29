package scenario

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/worklingo/backend/db/migrations"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

func TestFocusedAtomicResumeAndCap(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("scratch DB required")
	}
	if raw != "postgres://worklingo:worklingo@localhost:5433/worklingo_codex_20261029?sslmode=disable" {
		t.Fatal("refusing non-scratch DB")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	_ = goose.SetDialect("postgres")
	if err = goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := ws.NewHub(log, nil)
	for _, minutes := range []int{10, 20, 60} {
		t.Run(fmt.Sprintf("%dm", minutes), func(t *testing.T) {
			uid := uuid.New().String()
			u, err := q.CreateUser(ctx, store.CreateUserParams{Username: uid, Email: uid + "@test.local", PasswordHash: "unused"})
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Exec(ctx, "DELETE FROM users WHERE id=$1", u.ID)
			goal := "STATUS_UPDATE"
			due := time.Now().Add(-time.Second)
			sess, err := q.CreateSession(ctx, store.CreateSessionParams{UserID: u.ID, ExpiresAt: time.Now().Add(time.Duration(minutes) * time.Minute), LearningGoal: &goal, NextTaskAt: &due})
			if err != nil {
				t.Fatal(err)
			}
			_, err = q.CreateChannel(ctx, store.CreateChannelParams{SessionID: sess.ID, Kind: "CHANNEL", Name: "general"})
			if err != nil {
				t.Fatal(err)
			}
			s := New(Config{}, pool, q, nil, nil, nil, hub, nil, "", log)
			// Simulate a crash at schedule advancement, after message/task/points inserts.
			name := "focused_crash_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			_, err = pool.Exec(ctx, "CREATE FUNCTION "+name+`() RETURNS trigger AS $$ BEGIN IF NEW.id::text = TG_ARGV[0] THEN RAISE EXCEPTION 'simulated crash'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql; CREATE TRIGGER `+name+" BEFORE UPDATE ON sessions FOR EACH ROW EXECUTE FUNCTION "+name+"('"+sess.ID.String()+"')")
			if err != nil {
				t.Fatal(err)
			}
			remove := func() {
				_, _ = pool.Exec(context.Background(), "DROP TRIGGER IF EXISTS "+name+" ON sessions; DROP FUNCTION IF EXISTS "+name+"()")
			}
			defer remove()
			if err = s.fireFocused(ctx, sess.ID); err == nil {
				t.Fatal("crash hook did not fail")
			}
			messages, _ := q.ListSessionMessages(ctx, sess.ID)
			taskRows, _ := q.ListTasks(ctx, sess.ID)
			pointRows, _ := q.ListTalkingPointsForSession(ctx, sess.ID)
			row, _ := q.GetSession(ctx, sess.ID)
			if len(messages)+len(taskRows)+len(pointRows) != 0 || row.FocusedStep != 0 {
				t.Fatal("crash left partial task")
			}
			remove()
			// Recovery is overdue for every remaining template; never flood catch-up.
			_, err = pool.Exec(ctx, "UPDATE sessions SET started_at=now()-make_interval(mins=>$2),expires_at=now()+interval '1 minute' WHERE id=$1", sess.ID, minutes-1)
			if err != nil {
				t.Fatal(err)
			}
			limit := 2
			if minutes > 10 {
				limit = 3
			}
			for step := 0; step < limit; step++ {
				var wg sync.WaitGroup
				for i := 0; i < 12; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if err := s.fireFocused(ctx, sess.ID); err != nil {
							t.Error(err)
						}
					}()
				}
				wg.Wait()
				tasks, _ := q.ListTasks(ctx, sess.ID)
				if len(tasks) != step+1 {
					t.Fatalf("concurrent duplicate or missing tasks: %d", len(tasks))
				}
				pts, _ := q.ListTalkingPointsForSession(ctx, sess.ID)
				row, _ := q.GetSession(ctx, sess.ID)
				if step < limit-1 && (row.NextTaskAt == nil || time.Until(*row.NextTaskAt) < time.Second) {
					t.Fatal("catch-up tasks not spaced")
				}
				if len(pts) != 2*(step+1) {
					t.Fatal("partial transaction")
				}
				if step < limit-1 {
					_, err = pool.Exec(ctx, "UPDATE sessions SET next_task_at=now()-interval '1 second' WHERE id=$1", sess.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
				s = New(Config{}, pool, q, nil, nil, nil, hub, nil, "", log)
			}
			for i := 0; i < 3; i++ {
				if err = s.fireFocused(ctx, sess.ID); err != nil {
					t.Fatal(err)
				}
			}
			row, _ = q.GetSession(ctx, sess.ID)
			if row.NextTaskAt != nil || int(row.FocusedStep) != limit {
				t.Fatalf("exhaustion %+v", row)
			}
			ds, _ := q.ListDeadlines(ctx, sess.ID)
			if len(ds) != 0 {
				t.Fatal("focused deadlines")
			}
			_, err = pool.Exec(ctx, "UPDATE sessions SET focused_step=0,next_task_at=now()-interval '1 second',expires_at=now()-interval '1 second' WHERE id=$1", sess.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.fireFocused(ctx, sess.ID); err != nil {
				t.Fatal(err)
			}
			expiredTasks, _ := q.ListTasks(ctx, sess.ID)
			if len(expiredTasks) != limit {
				t.Fatal("expired session generated a task")
			}

		})
	}
}
