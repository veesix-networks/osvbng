package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/veesix-networks/osvbng/pkg/opdb"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
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

func (s *Store) Put(ctx context.Context, namespace, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO opdb (namespace, key, value, updated_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))
		ON CONFLICT(namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, namespace, key, value)
	return err
}

func (s *Store) Delete(ctx context.Context, namespace, key string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM opdb WHERE namespace = ? AND key = ?
	`, namespace, key)
	return err
}

func (s *Store) Load(ctx context.Context, namespace string, fn opdb.LoadFunc) error {
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

func (s *Store) Clear(ctx context.Context, namespace string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM opdb WHERE namespace = ?
	`, namespace)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
