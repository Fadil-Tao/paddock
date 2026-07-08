package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func InitDb(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)

	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	_, err = db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS sandbox (
			id TEXT PRIMARY KEY,
			container_id TEXT,
			name TEXT NOT NULL,
			state TEXT NOT NULL,
			image TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_exec TEXT,

			terminal_port TEXT,
			vnc_port TEXT,
			cdp_port TEXT,

			runtime TEXT,
			network_id TEXT,
			volume_name TEXT,

			vcpu TEXT,
			ram TEXT,
			storage TEXT,

			auto_stop TEXT,
			auto_archive TEXT,
			auto_delete TEXT
		)`,
	)

	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
