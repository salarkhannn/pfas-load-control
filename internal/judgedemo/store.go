package judgedemo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salarkhannn/pfas-load-control/internal/database/db"
)

type runStore interface {
	AcquireIdempotency(context.Context, string) (func(), error)
	Get(context.Context, string) (DemoRun, error)
	GetByIdempotencyKey(context.Context, string) (DemoRun, error)
	Save(context.Context, string, DemoRun) (DemoRun, error)
}

type postgresStore struct{ pool *pgxpool.Pool }

func (s postgresStore) AcquireIdempotency(ctx context.Context, key string) (func(), error) {
	connection, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire judge-demo idempotency connection: %w", err)
	}
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended($1, 0))", key); err != nil {
		connection.Release()
		return nil, fmt.Errorf("lock judge-demo idempotency key: %w", err)
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := connection.Exec(releaseCtx, "SELECT pg_advisory_unlock(hashtextextended($1, 0))", key); err != nil {
			_ = connection.Conn().Close(releaseCtx)
		}
		connection.Release()
	}, nil
}

func (s postgresStore) Get(ctx context.Context, id string) (DemoRun, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return DemoRun{}, ErrNotFound
	}
	record, err := db.New(s.pool).GetJudgeDemoRun(ctx, parsed)
	return hydrateStoredRun(record, err)
}

func (s postgresStore) GetByIdempotencyKey(ctx context.Context, key string) (DemoRun, error) {
	record, err := db.New(s.pool).GetJudgeDemoRunByIdempotencyKey(ctx, key)
	return hydrateStoredRun(record, err)
}

func (s postgresStore) Save(ctx context.Context, key string, run DemoRun) (DemoRun, error) {
	encoded, err := json.Marshal(run)
	if err != nil {
		return DemoRun{}, fmt.Errorf("encode judge-demo run: %w", err)
	}
	record, err := db.New(s.pool).CreateJudgeDemoRun(ctx, db.CreateJudgeDemoRunParams{
		ID: uuid.MustParse(run.ID), IdempotencyKey: key, Status: run.RunStatus,
		FixtureVersion: run.FixtureVersion, Record: encoded, DecisionHash: run.Package.DecisionHash,
		PackageArtifact: append([]byte(nil), run.Package.Artifact...),
		CreatedAt:       pgtype.Timestamptz{Time: run.CreatedAt, Valid: true},
		CompletedAt:     pgtype.Timestamptz{Time: run.CompletedAt, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.GetByIdempotencyKey(ctx, key)
	}
	return hydrateStoredRun(record, err)
}

func hydrateStoredRun(record db.PfasJudgeDemoRun, err error) (DemoRun, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return DemoRun{}, ErrNotFound
	}
	if err != nil {
		return DemoRun{}, fmt.Errorf("load judge-demo run: %w", err)
	}
	var run DemoRun
	if err := json.Unmarshal(record.Record, &run); err != nil {
		return DemoRun{}, fmt.Errorf("decode judge-demo run: %w", err)
	}
	run.Package.Artifact = append([]byte(nil), record.PackageArtifact...)
	if run.ID != record.ID.String() || run.Package.DecisionHash != record.DecisionHash || run.RunStatus != record.Status {
		return DemoRun{}, errors.New("stored judge-demo integrity check failed")
	}
	return verifyRun(run)
}

func verifyRun(run DemoRun) (DemoRun, error) {
	if len(run.Package.Artifact) == 0 || hashBytes(run.Package.Artifact) != run.Package.DecisionHash {
		return DemoRun{}, ErrIntegrity
	}
	return run, nil
}

type memoryStore struct {
	mu     sync.RWMutex
	runs   map[string]DemoRun
	byKeys map[string]string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{runs: make(map[string]DemoRun), byKeys: make(map[string]string)}
}

func (s *memoryStore) AcquireIdempotency(_ context.Context, _ string) (func(), error) {
	return func() {}, nil
}

func (s *memoryStore) Get(_ context.Context, id string) (DemoRun, error) {
	s.mu.RLock()
	run, ok := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return DemoRun{}, ErrNotFound
	}
	return verifyRun(run)
}

func (s *memoryStore) GetByIdempotencyKey(_ context.Context, key string) (DemoRun, error) {
	s.mu.RLock()
	id, ok := s.byKeys[key]
	run := s.runs[id]
	s.mu.RUnlock()
	if !ok {
		return DemoRun{}, ErrNotFound
	}
	return verifyRun(run)
}

func (s *memoryStore) Save(_ context.Context, key string, run DemoRun) (DemoRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byKeys[key]; ok {
		return verifyRun(s.runs[id])
	}
	s.runs[run.ID] = run
	s.byKeys[key] = run.ID
	return verifyRun(run)
}
