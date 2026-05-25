package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/veesix-networks/osvbng/pkg/opdb"
)

const (
	writeRetryAttempts = 4
	writeRetryBaseWait = 10 * time.Millisecond
)

type Store struct {
	db      *sql.DB
	puts    atomic.Uint64
	deletes atomic.Uint64
	loads   atomic.Uint64
	clears  atomic.Uint64
	retries atomic.Uint64
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	dsn := buildDSN(path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS opdb (
			namespace TEXT NOT NULL,
			key TEXT NOT NULL,
			value BLOB NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			PRIMARY KEY (namespace, key)
		)
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_opdb_namespace ON opdb(namespace)`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// buildDSN encodes SQLite pragmas as DSN query parameters so they apply to
// every connection database/sql opens from the pool. PRAGMA statements
// issued via db.Exec only affect the connection they ran on, leaving newly-
// pooled connections without WAL / busy_timeout — which silently restores
// the lock-contention behaviour this code is meant to avoid.
func buildDSN(path string) string {
	params := url.Values{}
	params.Set("_journal_mode", "WAL")
	params.Set("_synchronous", "NORMAL")
	params.Set("_busy_timeout", "5000")
	params.Set("_cache_size", "-64000")
	return fmt.Sprintf("file:%s?%s", path, params.Encode())
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	var sErr sqlite3.Error
	if errors.As(err, &sErr) {
		return sErr.Code == sqlite3.ErrBusy || sErr.Code == sqlite3.ErrLocked
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked")
}

func (s *Store) execRetry(ctx context.Context, query string, args ...any) error {
	for attempt := 0; attempt < writeRetryAttempts; attempt++ {
		_, err := s.db.ExecContext(ctx, query, args...)
		if err == nil {
			return nil
		}
		if !isBusy(err) {
			return err
		}
		s.retries.Add(1)
		wait := writeRetryBaseWait << attempt
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) Put(ctx context.Context, namespace, key string, value []byte) error {
	err := s.execRetry(ctx, `
		INSERT INTO opdb (namespace, key, value, updated_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))
		ON CONFLICT(namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, namespace, key, value)
	if err == nil {
		s.puts.Add(1)
	}
	return err
}

func (s *Store) Delete(ctx context.Context, namespace, key string) error {
	err := s.execRetry(ctx, `
		DELETE FROM opdb WHERE namespace = ? AND key = ?
	`, namespace, key)
	if err == nil {
		s.deletes.Add(1)
	}
	return err
}

func (s *Store) Load(ctx context.Context, namespace string, fn opdb.LoadFunc) error {
	s.loads.Add(1)
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, value FROM opdb WHERE namespace = ?
	`, namespace)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		if err := fn(key, value); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) Count(ctx context.Context, namespace string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM opdb WHERE namespace = ?`, namespace).Scan(&count)
	return count, err
}

func (s *Store) Clear(ctx context.Context, namespace string) error {
	err := s.execRetry(ctx, `
		DELETE FROM opdb WHERE namespace = ?
	`, namespace)
	if err == nil {
		s.clears.Add(1)
	}
	return err
}

func (s *Store) Stats() opdb.Stats {
	return opdb.Stats{
		Puts:    s.puts.Load(),
		Deletes: s.deletes.Load(),
		Loads:   s.loads.Load(),
		Clears:  s.clears.Load(),
	}
}

func (s *Store) Close() error {
	return s.db.Close()
}
