package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Fadil-Tao/paddock/internal/model"
	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

func NewSandboxRepository(db *sql.DB) *DB {
	return &DB{db: db}
}

func (d *DB) Create(ctx context.Context, sandbox *model.Sandbox) error {
	const query = `
		INSERT INTO sandbox (
			id,
			container_id,
			name,
			state,
			image,
			created_at,
			last_exec,
			terminal_port,
			vnc_port,
			cdp_port,
			runtime,
			network_id,
			volume_name,
			vcpu,
			ram,
			storage,
			auto_archive,
			auto_stop,
			auto_delete
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.ExecContext(
		ctx,
		query,
		sandbox.ID,
		sandbox.InternalId,
		sandbox.Name,
		sandbox.State,
		sandbox.Image,
		sandbox.CreatedAt,
		sandbox.LastExecAt,
		sandbox.Ports.Terminal,
		sandbox.Ports.VNC,
		sandbox.Ports.CDP,
		sandbox.Runtime,
		sandbox.NetworkId,
		sandbox.VolumeName,
		sandbox.VCpu,
		sandbox.Ram,
		sandbox.Storage,
		sandbox.AutoArchive,
		sandbox.AutoStop,
		sandbox.AutoDelete,
	)

	if err != nil {
		return err
	}

	return nil
}

func (d *DB) List(ctx context.Context, params *model.SearchParams) (*[]model.Sandbox, error) {
	query := `
		SELECT
			id,
			container_id,
			name,
			state,
			image,
			created_at,
			last_exec,
			terminal_port,
			vnc_port,
			cdp_port,
			runtime,
			network_id,
			volume_name,
			vcpu,
			ram,
			storage,
			auto_archive,
			auto_stop,
			auto_delete
		FROM sandbox
	`

	query, keyword, limit, offset := searchParams(query, params)

	rows, err := d.querySandboxRows(ctx, query, keyword, limit, offset)
	if err != nil {
		log.Err(err).Msg("failed to list sandboxes")
		return nil, fmt.Errorf("database operation error: %w", err)
	}
	defer rows.Close()

	sandboxes := make([]model.Sandbox, 0)
	for rows.Next() {
		var sandbox model.Sandbox
		var state string
		var createdAt string
		var containerID sql.NullString
		var lastExec sql.NullString
		var terminalPort sql.NullString
		var vncPort sql.NullString
		var cdpPort sql.NullString
		var runtime sql.NullString
		var networkID sql.NullString
		var volumeName sql.NullString
		var vcpu sql.NullString
		var ram sql.NullString
		var storage sql.NullString
		var autoArchive sql.NullString
		var autoStop sql.NullString
		var autoDelete sql.NullString

		err := rows.Scan(
			&sandbox.ID,
			&containerID,
			&sandbox.Name,
			&state,
			&sandbox.Image,
			&createdAt,
			&lastExec,
			&terminalPort,
			&vncPort,
			&cdpPort,
			&runtime,
			&networkID,
			&volumeName,
			&vcpu,
			&ram,
			&storage,
			&autoArchive,
			&autoStop,
			&autoDelete,
		)
		if err != nil {
			log.Err(err).Msg("failed to scan sandbox")
			return nil, fmt.Errorf("database operation error: %w", err)
		}

		sandbox.State = model.SandboxState(state)
		sandbox.InternalId = containerID.String
		sandbox.Ports.Terminal = terminalPort.String
		sandbox.Ports.VNC = vncPort.String
		sandbox.Ports.CDP = cdpPort.String
		sandbox.Runtime = runtime.String
		sandbox.NetworkId = networkID.String
		sandbox.VolumeName = volumeName.String
		sandbox.VCpu = vcpu.String
		sandbox.Ram = ram.String
		sandbox.Storage = storage.String

		sandbox.CreatedAt, err = parseSQLiteTime(createdAt)
		if err != nil {
			log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox created_at")
			return nil, fmt.Errorf("database operation error: %w", err)
		}

		if lastExec.Valid && strings.TrimSpace(lastExec.String) != "" {
			sandbox.LastExecAt, err = parseSQLiteTime(lastExec.String)
			if err != nil {
				log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox last_exec")
				return nil, fmt.Errorf("database operation error: %w", err)
			}
		}
		if autoArchive.Valid && strings.TrimSpace(autoArchive.String) != "" {
			sandbox.AutoArchive, err = parseSQLiteTime(autoArchive.String)
			if err != nil {
				log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox auto_archive")
				return nil, fmt.Errorf("database operation error: %w", err)
			}
		}
		if autoStop.Valid && strings.TrimSpace(autoStop.String) != "" {
			sandbox.AutoStop, err = parseSQLiteTime(autoStop.String)
			if err != nil {
				log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox auto_stop")
				return nil, fmt.Errorf("database operation error: %w", err)
			}
		}
		if autoDelete.Valid && strings.TrimSpace(autoDelete.String) != "" {
			sandbox.AutoDelete, err = parseSQLiteTime(autoDelete.String)
			if err != nil {
				log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox auto_delete")
				return nil, fmt.Errorf("database operation error: %w", err)
			}
		}

		sandboxes = append(sandboxes, sandbox)
	}

	if err := rows.Err(); err != nil {
		log.Err(err).Msg("failed to iterate sandbox rows")
		return nil, fmt.Errorf("database operation error: %w", err)
	}

	return &sandboxes, nil
}

func (d *DB) querySandboxRows(ctx context.Context, query string, keyword string, limit int, offset int) (*sql.Rows, error) {
	hasKeyword := keyword != ""
	hasLimit := limit > 0

	switch {
	case hasKeyword && hasLimit:
		return d.db.QueryContext(ctx, query, keyword, keyword, keyword, limit, offset)
	case hasKeyword:
		return d.db.QueryContext(ctx, query, keyword, keyword, keyword)
	case hasLimit:
		return d.db.QueryContext(ctx, query, limit, offset)
	default:
		return d.db.QueryContext(ctx, query)
	}
}

func (d *DB) GetById(ctx context.Context, id string) (*model.Sandbox, error) {
	const query = `
		SELECT
			id,
			container_id,
			name,
			state,
			image,
			created_at,
			last_exec,
			terminal_port,
			vnc_port,
			cdp_port,
			runtime,
			network_id,
			volume_name,
			vcpu,
			ram,
			storage,
			auto_archive,
			auto_stop,
			auto_delete
		FROM sandbox
		WHERE id = ?
	`

	var sandbox model.Sandbox
	var state string
	var createdAt string
	var containerID sql.NullString
	var lastExec sql.NullString
	var terminalPort sql.NullString
	var vncPort sql.NullString
	var cdpPort sql.NullString
	var runtime sql.NullString
	var networkID sql.NullString
	var volumeName sql.NullString
	var vcpu sql.NullString
	var ram sql.NullString
	var storage sql.NullString
	var autoArchive sql.NullString
	var autoStop sql.NullString
	var autoDelete sql.NullString

	err := d.db.QueryRowContext(ctx, query, id).Scan(
		&sandbox.ID,
		&containerID,
		&sandbox.Name,
		&state,
		&sandbox.Image,
		&createdAt,
		&lastExec,
		&terminalPort,
		&vncPort,
		&cdpPort,
		&runtime,
		&networkID,
		&volumeName,
		&vcpu,
		&ram,
		&storage,
		&autoArchive,
		&autoStop,
		&autoDelete,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrSandboxNotFound
		}
		log.Err(err).Str("id", id).Msg("failed to get sandbox")
		return nil, fmt.Errorf("database operation error: %w", err)
	}

	sandbox.State = model.SandboxState(state)
	sandbox.InternalId = containerID.String
	sandbox.Ports.Terminal = terminalPort.String
	sandbox.Ports.VNC = vncPort.String
	sandbox.Ports.CDP = cdpPort.String
	sandbox.Runtime = runtime.String
	sandbox.NetworkId = networkID.String
	sandbox.VolumeName = volumeName.String
	sandbox.VCpu = vcpu.String
	sandbox.Ram = ram.String
	sandbox.Storage = storage.String

	sandbox.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox created_at")
		return nil, fmt.Errorf("database operation error: %w", err)
	}

	if lastExec.Valid && strings.TrimSpace(lastExec.String) != "" {
		sandbox.LastExecAt, err = parseSQLiteTime(lastExec.String)
		if err != nil {
			log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox last_exec")
			return nil, fmt.Errorf("database operation error: %w", err)
		}
	}
	if autoArchive.Valid && strings.TrimSpace(autoArchive.String) != "" {
		sandbox.AutoArchive, err = parseSQLiteTime(autoArchive.String)
		if err != nil {
			log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox auto_archive")
			return nil, fmt.Errorf("database operation error: %w", err)
		}
	}
	if autoStop.Valid && strings.TrimSpace(autoStop.String) != "" {
		sandbox.AutoStop, err = parseSQLiteTime(autoStop.String)
		if err != nil {
			log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox auto_stop")
			return nil, fmt.Errorf("database operation error: %w", err)
		}
	}
	if autoDelete.Valid && strings.TrimSpace(autoDelete.String) != "" {
		sandbox.AutoDelete, err = parseSQLiteTime(autoDelete.String)
		if err != nil {
			log.Err(err).Str("id", sandbox.ID).Msg("failed to parse sandbox auto_delete")
			return nil, fmt.Errorf("database operation error: %w", err)
		}
	}

	return &sandbox, nil
}

func (d *DB) Remove(ctx context.Context, id string) error {
	result, err := d.db.ExecContext(ctx, `DELETE FROM sandbox WHERE id = ?`, id)
	if err != nil {
		log.Err(err).Str("id", id).Msg("failed to remove sandbox")
		return fmt.Errorf("database operation error: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		log.Err(err).Str("id", id).Msg("failed to check removed sandbox")
		return fmt.Errorf("database operation error: %w", err)
	}
	if affected == 0 {
		return model.ErrSandboxNotFound
	}

	return nil
}

func (d *DB) Update(ctx context.Context, sandbox *model.Sandbox) error {
	const query = `
		UPDATE sandbox
		SET
			container_id = ?,
			name = ?,
			state = ?,
			image = ?,
			created_at = ?,
			last_exec = ?,
			terminal_port = ?,
			vnc_port = ?,
			cdp_port = ?,
			runtime = ?,
			network_id = ?,
			volume_name = ?,
			vcpu = ?,
			ram = ?,
			storage = ?,
			auto_archive = ?,
			auto_stop = ?,
			auto_delete = ?
		WHERE id = ?
	`

	result, err := d.db.ExecContext(
		ctx,
		query,
		sandbox.InternalId,
		sandbox.Name,
		sandbox.State,
		sandbox.Image,
		sandbox.CreatedAt,
		sandbox.LastExecAt,
		sandbox.Ports.Terminal,
		sandbox.Ports.VNC,
		sandbox.Ports.CDP,
		sandbox.Runtime,
		sandbox.NetworkId,
		sandbox.VolumeName,
		sandbox.VCpu,
		sandbox.Ram,
		sandbox.Storage,
		sandbox.AutoArchive,
		sandbox.AutoStop,
		sandbox.AutoDelete,
		sandbox.ID,
	)
	if err != nil {
		log.Err(err).Str("id", sandbox.ID).Msg("failed to update sandbox")
		return fmt.Errorf("database operation error: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		log.Err(err).Str("id", sandbox.ID).Msg("failed to check updated sandbox")
		return fmt.Errorf("database operation error: %w", err)
	}
	if affected == 0 {
		return model.ErrSandboxNotFound
	}

	return nil
}

func (d *DB) Patch(ctx context.Context, id string, patch *model.SandboxPatch) error {
	query, args, err := buildPatchQuery("sandbox", "id", id, patch)
	if err != nil {
		return fmt.Errorf("database operation error: %w", err)
	}
	if query == "" {
		_, err := d.GetById(ctx, id)
		return err
	}

	result, err := d.db.ExecContext(ctx, query, args...)
	if err != nil {
		log.Err(err).Str("id", id).Msg("failed to patch sandbox")
		return fmt.Errorf("database operation error: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		log.Err(err).Str("id", id).Msg("failed to check patched sandbox")
		return fmt.Errorf("database operation error: %w", err)
	}
	if affected == 0 {
		return model.ErrSandboxNotFound
	}

	return nil
}
