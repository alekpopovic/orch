package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	*postgresStore
	pool *pgxpool.Pool
}

type postgresStore struct {
	db    postgresDB
	begin func(context.Context) (pgx.Tx, error)
}

type postgresDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{
		postgresStore: &postgresStore{
			db:    pool,
			begin: pool.Begin,
		},
		pool: pool,
	}
}

func (s *PostgresStore) WithTx(ctx context.Context, fn TxFunc) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("%w: postgres store is not initialized", ErrInvalidState)
	}
	return s.postgresStore.withTx(ctx, func(txStore *postgresStore) error {
		return fn(ctx, txStore)
	})
}

func ConnectPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func (s *postgresStore) withTx(ctx context.Context, fn func(*postgresStore) error) error {
	if s.begin == nil {
		return fn(s)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return mapPostgresError(err)
	}
	txStore := &postgresStore{db: tx}
	defer rollback(ctx, tx)
	if err := fn(txStore); err != nil {
		return err
	}
	return mapPostgresError(tx.Commit(ctx))
}

func (s *postgresStore) CreateNode(ctx context.Context, spec types.NodeSpec) (types.Node, error) {
	if err := spec.Validate(); err != nil {
		return types.Node{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}

	labels, err := jsonBytes(defaultMap(spec.Labels))
	if err != nil {
		return types.Node{}, err
	}

	row := s.db.QueryRow(ctx, `
		INSERT INTO nodes (
			hostname, advertise_address, labels,
			capacity_cpu, capacity_memory, allocatable_cpu, allocatable_memory,
			status, last_heartbeat_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, timezone('utc', now()))
		RETURNING id, hostname, advertise_address, labels,
			capacity_cpu, capacity_memory, allocatable_cpu, allocatable_memory,
			status, last_heartbeat_at, agent_token_hash, agent_token_expires_at, agent_revoked, created_at, updated_at`,
		spec.Hostname,
		spec.AdvertiseAddress,
		labels,
		spec.Capacity.CPU,
		spec.Capacity.Memory,
		spec.Allocatable.CPU,
		spec.Allocatable.Memory,
		string(types.NodeUnknown),
	)
	node, err := scanNode(row)
	if err != nil {
		return types.Node{}, mapPostgresError(err)
	}
	return node, nil
}

func (s *postgresStore) GetNode(ctx context.Context, id types.NodeID) (types.Node, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, hostname, advertise_address, labels,
			capacity_cpu, capacity_memory, allocatable_cpu, allocatable_memory,
			status, last_heartbeat_at, agent_token_hash, agent_token_expires_at, agent_revoked, created_at, updated_at
		FROM nodes
		WHERE id = $1`,
		string(id),
	)
	node, err := scanNode(row)
	if err != nil {
		return types.Node{}, mapPostgresError(err)
	}
	return node, nil
}

func (s *postgresStore) UpdateNode(ctx context.Context, node types.Node, expectedUpdatedAt time.Time) (types.Node, error) {
	spec := types.NodeSpec{
		Hostname:         node.Hostname,
		AdvertiseAddress: node.AdvertiseAddress,
		Labels:           node.Labels,
		Capacity:         node.Capacity,
		Allocatable:      node.Allocatable,
	}
	if err := spec.Validate(); err != nil {
		return types.Node{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}

	labels, err := jsonBytes(defaultMap(node.Labels))
	if err != nil {
		return types.Node{}, err
	}

	row := s.db.QueryRow(ctx, `
		UPDATE nodes
		SET hostname = $2,
			advertise_address = $3,
			labels = $4,
			capacity_cpu = $5,
			capacity_memory = $6,
			allocatable_cpu = $7,
			allocatable_memory = $8,
			status = $9,
			last_heartbeat_at = $10,
			agent_token_hash = nullif($12, ''),
			agent_token_expires_at = $13,
			agent_revoked = $14,
			updated_at = timezone('utc', now()),
			version = version + 1
		WHERE id = $1 AND updated_at = $11
		RETURNING id, hostname, advertise_address, labels,
			capacity_cpu, capacity_memory, allocatable_cpu, allocatable_memory,
			status, last_heartbeat_at, agent_token_hash, agent_token_expires_at, agent_revoked, created_at, updated_at`,
		string(node.ID),
		node.Hostname,
		node.AdvertiseAddress,
		labels,
		node.Capacity.CPU,
		node.Capacity.Memory,
		node.Allocatable.CPU,
		node.Allocatable.Memory,
		string(node.Status),
		utcOrNow(node.LastHeartbeatAt),
		expectedUpdatedAt.UTC(),
		node.AgentTokenHash,
		nilTime(node.AgentTokenExpiry),
		node.AgentRevoked,
	)
	updated, err := scanNode(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Node{}, ErrConflict
		}
		return types.Node{}, mapPostgresError(err)
	}
	return updated, nil
}

func (s *postgresStore) ListNodesByStatus(ctx context.Context, status types.NodeStatus) ([]types.Node, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, hostname, advertise_address, labels,
			capacity_cpu, capacity_memory, allocatable_cpu, allocatable_memory,
			status, last_heartbeat_at, agent_token_hash, agent_token_expires_at, agent_revoked, created_at, updated_at
		FROM nodes
		WHERE status = $1
		ORDER BY hostname, id`,
		string(status),
	)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	defer rows.Close()

	var nodes []types.Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, mapPostgresError(err)
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}
	return nodes, nil
}

func (s *postgresStore) CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error) {
	normalized, err := types.NormalizeServiceSpec(spec, types.DefaultResourceDefaults())
	if err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	spec = normalized
	if err := spec.Validate(); err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}

	var service types.Service
	err = s.withTx(ctx, func(tx *postgresStore) error {
		created, err := insertService(ctx, tx.db, spec)
		if err != nil {
			return err
		}
		if err := insertServiceVersion(ctx, tx.db, created.ID, created.DeploymentVersion, spec); err != nil {
			return err
		}
		service = created
		return nil
	})
	if err != nil {
		return types.Service{}, err
	}
	return service, nil
}

func (s *postgresStore) GetService(ctx context.Context, id types.ServiceID) (types.Service, error) {
	row := s.db.QueryRow(ctx, serviceSelectSQL()+` WHERE id = $1`, string(id))
	service, err := scanService(row)
	if err != nil {
		return types.Service{}, mapPostgresError(err)
	}
	return service, nil
}

func (s *postgresStore) ListServices(ctx context.Context) ([]types.Service, error) {
	rows, err := s.db.Query(ctx, serviceSelectSQL()+` ORDER BY name, id`)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	defer rows.Close()

	var services []types.Service
	for rows.Next() {
		service, err := scanService(rows)
		if err != nil {
			return nil, mapPostgresError(err)
		}
		services = append(services, service)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}
	return services, nil
}

func (s *postgresStore) UpdateService(ctx context.Context, id types.ServiceID, spec types.ServiceSpec, expectedUpdatedAt time.Time) (types.Service, error) {
	normalized, err := types.NormalizeServiceSpec(spec, types.DefaultResourceDefaults())
	if err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	spec = normalized
	if err := spec.Validate(); err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}

	var service types.Service
	err = s.withTx(ctx, func(tx *postgresStore) error {
		updated, err := updateService(ctx, tx.db, id, spec, expectedUpdatedAt)
		if err != nil {
			return err
		}
		if err := insertServiceVersion(ctx, tx.db, updated.ID, updated.DeploymentVersion, spec); err != nil {
			return err
		}
		service = updated
		return nil
	})
	if err != nil {
		return types.Service{}, err
	}
	return service, nil
}

func (s *postgresStore) UpdateServiceStatus(ctx context.Context, id types.ServiceID, status types.ServiceStatus, expectedUpdatedAt time.Time) (types.Service, error) {
	current, err := s.GetService(ctx, id)
	if err != nil {
		return types.Service{}, err
	}
	if err := types.ValidateServiceTransition(current.Status, status); err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	row := s.db.QueryRow(ctx, `
		UPDATE services
		SET status = $2,
			updated_at = timezone('utc', now()),
			version = version + 1
		WHERE id = $1 AND updated_at = $3
		RETURNING id, name, image, image_pull_secret, replicas, env, secret_refs, ports,
			resource_requirements, healthcheck, restart_policy, placement_constraints, routes,
			status, deployment_version, created_at, updated_at`,
		string(id),
		string(status),
		expectedUpdatedAt.UTC(),
	)
	service, err := scanService(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Service{}, ErrConflict
		}
		return types.Service{}, mapPostgresError(err)
	}
	return service, nil
}

func (s *postgresStore) CreateSecret(ctx context.Context, secret types.Secret) (types.Secret, error) {
	secret.Name = strings.TrimSpace(secret.Name)
	if secret.Name == "" {
		return types.Secret{}, fmt.Errorf("%w: secret name is required", ErrInvalidState)
	}
	if len(secret.EncryptedValue) == 0 {
		return types.Secret{}, fmt.Errorf("%w: encrypted secret value is required", ErrInvalidState)
	}
	if strings.TrimSpace(secret.KeyID) == "" {
		return types.Secret{}, fmt.Errorf("%w: secret key id is required", ErrInvalidState)
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO secrets (name, encrypted_value, key_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE
		SET encrypted_value = EXCLUDED.encrypted_value,
			key_id = EXCLUDED.key_id,
			updated_at = timezone('utc', now())
		RETURNING name, encrypted_value, key_id, created_at, updated_at`,
		secret.Name,
		secret.EncryptedValue,
		secret.KeyID,
	)
	created, err := scanSecret(row)
	if err != nil {
		return types.Secret{}, mapPostgresError(err)
	}
	return created, nil
}

func (s *postgresStore) GetSecret(ctx context.Context, name string) (types.Secret, error) {
	row := s.db.QueryRow(ctx, `
		SELECT name, encrypted_value, key_id, created_at, updated_at
		FROM secrets
		WHERE name = $1`,
		strings.TrimSpace(name),
	)
	secret, err := scanSecret(row)
	if err != nil {
		return types.Secret{}, mapPostgresError(err)
	}
	return secret, nil
}

func (s *postgresStore) ListSecrets(ctx context.Context) ([]types.Secret, error) {
	rows, err := s.db.Query(ctx, `
		SELECT name, encrypted_value, key_id, created_at, updated_at
		FROM secrets
		ORDER BY name`)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	defer rows.Close()

	secrets := make([]types.Secret, 0)
	for rows.Next() {
		secret, err := scanSecret(rows)
		if err != nil {
			return nil, mapPostgresError(err)
		}
		secrets = append(secrets, secret)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}
	return secrets, nil
}

func (s *postgresStore) DeleteSecret(ctx context.Context, name string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM secrets WHERE name = $1`, strings.TrimSpace(name))
	if err != nil {
		return mapPostgresError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *postgresStore) CreateRegistryCredential(ctx context.Context, credential types.RegistryCredential) (types.RegistryCredential, error) {
	credential.ID = strings.TrimSpace(credential.ID)
	credential.Registry = strings.TrimSpace(credential.Registry)
	credential.Username = strings.TrimSpace(credential.Username)
	if credential.ID == "" {
		return types.RegistryCredential{}, fmt.Errorf("%w: registry credential id is required", ErrInvalidState)
	}
	if credential.Registry == "" {
		return types.RegistryCredential{}, fmt.Errorf("%w: registry host is required", ErrInvalidState)
	}
	if credential.Username == "" {
		return types.RegistryCredential{}, fmt.Errorf("%w: registry username is required", ErrInvalidState)
	}
	if len(credential.EncryptedPassword) == 0 {
		return types.RegistryCredential{}, fmt.Errorf("%w: encrypted registry credential password is required", ErrInvalidState)
	}
	if strings.TrimSpace(credential.KeyID) == "" {
		return types.RegistryCredential{}, fmt.Errorf("%w: registry credential key id is required", ErrInvalidState)
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO registry_credentials (id, registry, username, encrypted_password, key_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE
		SET registry = EXCLUDED.registry,
			username = EXCLUDED.username,
			encrypted_password = EXCLUDED.encrypted_password,
			key_id = EXCLUDED.key_id,
			updated_at = timezone('utc', now())
		RETURNING id, registry, username, encrypted_password, key_id, created_at, updated_at`,
		credential.ID,
		credential.Registry,
		credential.Username,
		credential.EncryptedPassword,
		credential.KeyID,
	)
	created, err := scanRegistryCredential(row)
	if err != nil {
		return types.RegistryCredential{}, mapPostgresError(err)
	}
	return created, nil
}

func (s *postgresStore) GetRegistryCredential(ctx context.Context, id string) (types.RegistryCredential, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, registry, username, encrypted_password, key_id, created_at, updated_at
		FROM registry_credentials
		WHERE id = $1`,
		strings.TrimSpace(id),
	)
	credential, err := scanRegistryCredential(row)
	if err != nil {
		return types.RegistryCredential{}, mapPostgresError(err)
	}
	return credential, nil
}

func (s *postgresStore) ListRegistryCredentials(ctx context.Context) ([]types.RegistryCredential, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, registry, username, encrypted_password, key_id, created_at, updated_at
		FROM registry_credentials
		ORDER BY id`)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	defer rows.Close()

	credentials := make([]types.RegistryCredential, 0)
	for rows.Next() {
		credential, err := scanRegistryCredential(rows)
		if err != nil {
			return nil, mapPostgresError(err)
		}
		credentials = append(credentials, credential)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}
	return credentials, nil
}

func (s *postgresStore) DeleteRegistryCredential(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM registry_credentials WHERE id = $1`, strings.TrimSpace(id))
	if err != nil {
		return mapPostgresError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *postgresStore) CreateTask(ctx context.Context, task types.Task) (types.Task, error) {
	if task.DesiredStatus == "" {
		task.DesiredStatus = types.TaskRunning
	}
	if task.ActualStatus == "" {
		task.ActualStatus = types.TaskPending
	}
	if !types.ValidTaskStatus(task.DesiredStatus) || !types.ValidTaskStatus(task.ActualStatus) {
		return types.Task{}, fmt.Errorf("%w: task status is invalid", ErrInvalidState)
	}
	ports, err := jsonBytes(defaultSlice(task.Ports))
	if err != nil {
		return types.Task{}, err
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO tasks (
			service_id, node_id, container_id, desired_status, actual_status,
			image, version, ports, restart_count, failure_reason, started_at, finished_at
		)
		VALUES ($1, nullif($2, '')::uuid, nullif($3, ''), $4, $5, $6, $7, $8, $9, nullif($10, ''), $11, $12)
		RETURNING id, service_id, node_id, container_id, desired_status, actual_status,
			image, version, ports, restart_count, failure_reason, created_at, updated_at, started_at, finished_at`,
		string(task.ServiceID),
		string(task.NodeID),
		task.ContainerID,
		string(task.DesiredStatus),
		string(task.ActualStatus),
		task.Image,
		task.Version,
		ports,
		task.RestartCount,
		task.FailureReason,
		nilTime(task.StartedAt),
		nilTime(task.FinishedAt),
	)
	created, err := scanTask(row)
	if err != nil {
		return types.Task{}, mapPostgresError(err)
	}
	return created, nil
}

func (s *postgresStore) GetTask(ctx context.Context, id types.TaskID) (types.Task, error) {
	return s.getTask(ctx, id, false)
}

func (s *postgresStore) getTask(ctx context.Context, id types.TaskID, forUpdate bool) (types.Task, error) {
	sql := taskSelectSQL() + ` WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE SKIP LOCKED`
	}
	row := s.db.QueryRow(ctx, sql, string(id))
	task, err := scanTask(row)
	if err != nil {
		return types.Task{}, mapPostgresError(err)
	}
	return task, nil
}

func (s *postgresStore) AssignTask(ctx context.Context, id types.TaskID, nodeID types.NodeID, ports []types.Port, expectedUpdatedAt time.Time) (types.Task, error) {
	current, err := s.getTask(ctx, id, s.begin == nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) && s.begin == nil {
			if _, getErr := s.getTask(ctx, id, false); errors.Is(getErr, ErrNotFound) {
				return types.Task{}, ErrNotFound
			} else if getErr != nil {
				return types.Task{}, getErr
			}
			return types.Task{}, ErrConflict
		}
		return types.Task{}, err
	}
	if err := types.ValidateTaskTransition(current.ActualStatus, types.TaskAssigned); err != nil {
		return types.Task{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	portBytes, err := jsonBytes(defaultSlice(ports))
	if err != nil {
		return types.Task{}, err
	}
	row := s.db.QueryRow(ctx, `
		UPDATE tasks
		SET node_id = $2,
			actual_status = $3,
			ports = $4,
			updated_at = timezone('utc', now()),
			row_version = row_version + 1
		WHERE id = $1 AND updated_at = $5 AND actual_status = 'pending'
		RETURNING id, service_id, node_id, container_id, desired_status, actual_status,
			image, version, ports, restart_count, failure_reason, created_at, updated_at, started_at, finished_at`,
		string(id),
		string(nodeID),
		string(types.TaskAssigned),
		portBytes,
		expectedUpdatedAt.UTC(),
	)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Task{}, ErrConflict
		}
		return types.Task{}, mapPostgresError(err)
	}
	return task, nil
}

func (s *postgresStore) StopTask(ctx context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error) {
	current, err := s.GetTask(ctx, id)
	if err != nil {
		return types.Task{}, err
	}
	if current.DesiredStatus == types.TaskStopped || current.DesiredStatus == types.TaskRemoved {
		return current, nil
	}
	if err := types.ValidateTaskDesiredTransition(current.DesiredStatus, types.TaskStopped); err != nil {
		return types.Task{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	row := s.db.QueryRow(ctx, `
		UPDATE tasks
		SET desired_status = $2,
			updated_at = timezone('utc', now()),
			row_version = row_version + 1
		WHERE id = $1 AND updated_at = $3 AND desired_status <> $2
		RETURNING id, service_id, node_id, container_id, desired_status, actual_status,
			image, version, ports, restart_count, failure_reason, created_at, updated_at, started_at, finished_at`,
		string(id),
		string(types.TaskStopped),
		expectedUpdatedAt.UTC(),
	)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := s.GetTask(ctx, id)
			if getErr != nil {
				return types.Task{}, getErr
			}
			if current.DesiredStatus == types.TaskStopped || current.DesiredStatus == types.TaskRemoved {
				return current, nil
			}
			return types.Task{}, ErrConflict
		}
		return types.Task{}, mapPostgresError(err)
	}
	return task, nil
}

func (s *postgresStore) UpdateTaskStatus(ctx context.Context, id types.TaskID, desired types.TaskStatus, actual types.TaskStatus, containerID string, failureReason string, expectedUpdatedAt time.Time) (types.Task, error) {
	current, err := s.GetTask(ctx, id)
	if err != nil {
		return types.Task{}, err
	}
	if err := types.ValidateTaskDesiredTransition(current.DesiredStatus, desired); err != nil {
		return types.Task{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	if err := types.ValidateTaskTransition(current.ActualStatus, actual); err != nil {
		return types.Task{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	row := s.db.QueryRow(ctx, `
		UPDATE tasks
		SET desired_status = $2,
			actual_status = $3,
			container_id = nullif($4, ''),
			failure_reason = nullif($5, ''),
			updated_at = timezone('utc', now()),
			row_version = row_version + 1
		WHERE id = $1 AND updated_at = $6
		RETURNING id, service_id, node_id, container_id, desired_status, actual_status,
			image, version, ports, restart_count, failure_reason, created_at, updated_at, started_at, finished_at`,
		string(id),
		string(desired),
		string(actual),
		containerID,
		failureReason,
		expectedUpdatedAt.UTC(),
	)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Task{}, ErrConflict
		}
		return types.Task{}, mapPostgresError(err)
	}
	return task, nil
}

func (s *postgresStore) ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error) {
	return s.listTasks(ctx, taskSelectSQL()+` WHERE service_id = $1 ORDER BY created_at, id`, string(serviceID))
}

func (s *postgresStore) ListTasksByNode(ctx context.Context, nodeID types.NodeID) ([]types.Task, error) {
	return s.listTasks(ctx, taskSelectSQL()+` WHERE node_id = $1 ORDER BY created_at, id`, string(nodeID))
}

func (s *postgresStore) ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error) {
	return s.listTasks(ctx, taskSelectSQL()+` WHERE actual_status = $1 ORDER BY created_at, id`, string(status))
}

func (s *postgresStore) listTasks(ctx context.Context, sql string, arg any) ([]types.Task, error) {
	rows, err := s.db.Query(ctx, sql, arg)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	defer rows.Close()

	var tasks []types.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, mapPostgresError(err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}
	return tasks, nil
}

func (s *postgresStore) CreateDeployment(ctx context.Context, deployment types.Deployment) (types.Deployment, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO deployments (
			service_id, from_version, to_version, strategy, status,
			max_unavailable, max_surge, started_at, completed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, service_id, from_version, to_version, strategy, status,
			max_unavailable, max_surge, created_at, updated_at, started_at, completed_at`,
		string(deployment.ServiceID),
		deployment.FromVersion,
		deployment.ToVersion,
		string(deployment.Strategy),
		string(deployment.Status),
		deployment.MaxUnavailable,
		deployment.MaxSurge,
		nilTime(deployment.StartedAt),
		nilTime(deployment.CompletedAt),
	)
	created, err := scanDeployment(row)
	if err != nil {
		return types.Deployment{}, mapPostgresError(err)
	}
	return created, nil
}

func (s *postgresStore) GetDeployment(ctx context.Context, id types.DeploymentID) (types.Deployment, error) {
	row := s.db.QueryRow(ctx, deploymentSelectSQL()+` WHERE id = $1`, string(id))
	deployment, err := scanDeployment(row)
	if err != nil {
		return types.Deployment{}, mapPostgresError(err)
	}
	return deployment, nil
}

func (s *postgresStore) ListDeploymentsByStatus(ctx context.Context, status types.DeploymentStatus) ([]types.Deployment, error) {
	rows, err := s.db.Query(ctx, deploymentSelectSQL()+` WHERE status = $1 ORDER BY created_at, id`, string(status))
	if err != nil {
		return nil, mapPostgresError(err)
	}
	defer rows.Close()

	var deployments []types.Deployment
	for rows.Next() {
		deployment, err := scanDeployment(rows)
		if err != nil {
			return nil, mapPostgresError(err)
		}
		deployments = append(deployments, deployment)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}
	return deployments, nil
}

func (s *postgresStore) UpdateDeploymentStatus(ctx context.Context, id types.DeploymentID, status types.DeploymentStatus, expectedUpdatedAt time.Time) (types.Deployment, error) {
	current, err := s.GetDeployment(ctx, id)
	if err != nil {
		return types.Deployment{}, err
	}
	if err := types.ValidateDeploymentTransition(current.Status, status); err != nil {
		return types.Deployment{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	row := s.db.QueryRow(ctx, `
		UPDATE deployments
		SET status = $2,
			started_at = CASE WHEN $2 IN ('running', 'rolling_back') AND started_at IS NULL THEN timezone('utc', now()) ELSE started_at END,
			completed_at = CASE WHEN $2 IN ('succeeded', 'failed', 'paused', 'rolled_back') AND completed_at IS NULL THEN timezone('utc', now()) ELSE completed_at END,
			updated_at = timezone('utc', now()),
			row_version = row_version + 1
		WHERE id = $1 AND updated_at = $3
		RETURNING id, service_id, from_version, to_version, strategy, status,
			max_unavailable, max_surge, created_at, updated_at, started_at, completed_at`,
		string(id),
		string(status),
		expectedUpdatedAt.UTC(),
	)
	deployment, err := scanDeployment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Deployment{}, ErrConflict
		}
		return types.Deployment{}, mapPostgresError(err)
	}
	return deployment, nil
}

func (s *postgresStore) AppendEvent(ctx context.Context, event types.Event) (types.Event, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO events (
			type, severity, source, message, related_object_type, related_object_id, created_at
		)
		VALUES ($1, $2, $3, $4, nullif($5, ''), nullif($6, '')::uuid, COALESCE($7, timezone('utc', now())))
		RETURNING id, type, severity, source, message, related_object_type, related_object_id, created_at`,
		event.Type,
		string(event.Severity),
		event.Source,
		event.Message,
		event.RelatedObjectType,
		event.RelatedObjectID,
		nilTime(event.Timestamp),
	)
	created, err := scanEvent(row)
	if err != nil {
		return types.Event{}, mapPostgresError(err)
	}
	return created, nil
}

func (s *postgresStore) ListEvents(ctx context.Context, filter events.Filter) ([]types.Event, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	sql := `
		SELECT id, type, severity, source, message, related_object_type, related_object_id, created_at
		FROM events`
	args := make([]any, 0, 8)
	conditions := make([]string, 0, 8)
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if filter.ServiceID != "" {
		add("(related_object_type = 'service' AND related_object_id = $%d)", string(filter.ServiceID))
	}
	if filter.TaskID != "" {
		add("(related_object_type = 'task' AND related_object_id = $%d)", string(filter.TaskID))
	}
	if filter.NodeID != "" {
		add("(related_object_type = 'node' AND related_object_id = $%d)", string(filter.NodeID))
	}
	if filter.Type != "" {
		add("type = $%d", filter.Type)
	}
	if filter.Severity != "" {
		add("severity = $%d", string(filter.Severity))
	}
	if !filter.Since.IsZero() {
		add("created_at >= $%d", filter.Since.UTC())
	}
	if len(conditions) > 0 {
		sql += " WHERE " + joinConditions(conditions)
	}
	args = append(args, limit)
	sql += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	defer rows.Close()

	var events []types.Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, mapPostgresError(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}
	return events, nil
}

func insertService(ctx context.Context, db postgresDB, spec types.ServiceSpec) (types.Service, error) {
	env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints, routes, err := serviceJSON(spec)
	if err != nil {
		return types.Service{}, err
	}
	row := db.QueryRow(ctx, `
		INSERT INTO services (
			name, image, image_pull_secret, replicas, env, secret_refs, ports,
			resource_requirements, healthcheck, restart_policy, placement_constraints, routes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, name, image, image_pull_secret, replicas, env, secret_refs, ports,
			resource_requirements, healthcheck, restart_policy, placement_constraints, routes, status,
			deployment_version, created_at, updated_at`,
		spec.Name,
		spec.Image,
		spec.ImagePullSecret,
		spec.Replicas,
		env,
		secretRefs,
		ports,
		requirements,
		healthcheck,
		restartPolicy,
		constraints,
		routes,
	)
	service, err := scanService(row)
	if err != nil {
		return types.Service{}, mapPostgresError(err)
	}
	return service, nil
}

func updateService(ctx context.Context, db postgresDB, id types.ServiceID, spec types.ServiceSpec, expectedUpdatedAt time.Time) (types.Service, error) {
	env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints, routes, err := serviceJSON(spec)
	if err != nil {
		return types.Service{}, err
	}
	row := db.QueryRow(ctx, `
			UPDATE services
			SET name = $2,
				image = $3,
				image_pull_secret = $4,
				replicas = $5,
				env = $6,
				secret_refs = $7,
				ports = $8,
				resource_requirements = $9,
				healthcheck = $10,
				restart_policy = $11,
				placement_constraints = $12,
				routes = $14,
				deployment_version = deployment_version + 1,
				updated_at = timezone('utc', now()),
				version = version + 1
			WHERE id = $1 AND updated_at = $13
			RETURNING id, name, image, image_pull_secret, replicas, env, secret_refs, ports,
				resource_requirements, healthcheck, restart_policy, placement_constraints, routes,
				status, deployment_version, created_at, updated_at`,
		string(id),
		spec.Name,
		spec.Image,
		spec.ImagePullSecret,
		spec.Replicas,
		env,
		secretRefs,
		ports,
		requirements,
		healthcheck,
		restartPolicy,
		constraints,
		expectedUpdatedAt.UTC(),
		routes,
	)
	service, err := scanService(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Service{}, ErrConflict
		}
		return types.Service{}, mapPostgresError(err)
	}
	return service, nil
}

func insertServiceVersion(ctx context.Context, db postgresDB, serviceID types.ServiceID, version int64, spec types.ServiceSpec) error {
	specJSON, err := jsonBytes(spec)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO service_versions (service_id, version, spec)
		VALUES ($1, $2, $3)`,
		string(serviceID),
		version,
		specJSON,
	)
	return mapPostgresError(err)
}

func serviceSelectSQL() string {
	return `SELECT id, name, image, image_pull_secret, replicas, env, secret_refs, ports,
		resource_requirements, healthcheck, restart_policy, placement_constraints, routes,
		status, deployment_version, created_at, updated_at FROM services`
}

func taskSelectSQL() string {
	return `SELECT id, service_id, node_id, container_id, desired_status, actual_status,
		image, version, ports, restart_count, failure_reason, created_at, updated_at, started_at, finished_at FROM tasks`
}

func deploymentSelectSQL() string {
	return `SELECT id, service_id, from_version, to_version, strategy, status,
		max_unavailable, max_surge, created_at, updated_at, started_at, completed_at FROM deployments`
}

func joinConditions(conditions []string) string {
	return strings.Join(conditions, " AND ")
}

func scanNode(row pgx.Row) (types.Node, error) {
	var node types.Node
	var id string
	var labels []byte
	var tokenHash sql.NullString
	var tokenExpiry sql.NullTime
	err := row.Scan(
		&id,
		&node.Hostname,
		&node.AdvertiseAddress,
		&labels,
		&node.Capacity.CPU,
		&node.Capacity.Memory,
		&node.Allocatable.CPU,
		&node.Allocatable.Memory,
		&node.Status,
		&node.LastHeartbeatAt,
		&tokenHash,
		&tokenExpiry,
		&node.AgentRevoked,
		&node.CreatedAt,
		&node.UpdatedAt,
	)
	if err != nil {
		return types.Node{}, err
	}
	node.ID = types.NodeID(id)
	if err := json.Unmarshal(labels, &node.Labels); err != nil {
		return types.Node{}, fmt.Errorf("decode node labels: %w", err)
	}
	if tokenHash.Valid {
		node.AgentTokenHash = tokenHash.String
	}
	if tokenExpiry.Valid {
		node.AgentTokenExpiry = tokenExpiry.Time.UTC()
	}
	node.LastHeartbeatAt = node.LastHeartbeatAt.UTC()
	node.CreatedAt = node.CreatedAt.UTC()
	node.UpdatedAt = node.UpdatedAt.UTC()
	return node, nil
}

func scanService(row pgx.Row) (types.Service, error) {
	var service types.Service
	var id string
	var env, secretRefs, ports, requirements, restartPolicy, constraints, routes []byte
	var healthcheck []byte
	err := row.Scan(
		&id,
		&service.Spec.Name,
		&service.Spec.Image,
		&service.Spec.ImagePullSecret,
		&service.Spec.Replicas,
		&env,
		&secretRefs,
		&ports,
		&requirements,
		&healthcheck,
		&restartPolicy,
		&constraints,
		&routes,
		&service.Status,
		&service.DeploymentVersion,
		&service.CreatedAt,
		&service.UpdatedAt,
	)
	if err != nil {
		return types.Service{}, err
	}
	service.ID = types.ServiceID(id)
	if service.Status == "" {
		service.Status = types.ServiceActive
	}
	if err := decodeServiceJSON(&service.Spec, env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints, routes); err != nil {
		return types.Service{}, err
	}
	service.CreatedAt = service.CreatedAt.UTC()
	service.UpdatedAt = service.UpdatedAt.UTC()
	return service, nil
}

func scanTask(row pgx.Row) (types.Task, error) {
	var task types.Task
	var id, serviceID string
	var nodeID, containerID, failureReason *string
	var ports []byte
	var startedAt, finishedAt *time.Time
	err := row.Scan(
		&id,
		&serviceID,
		&nodeID,
		&containerID,
		&task.DesiredStatus,
		&task.ActualStatus,
		&task.Image,
		&task.Version,
		&ports,
		&task.RestartCount,
		&failureReason,
		&task.CreatedAt,
		&task.UpdatedAt,
		&startedAt,
		&finishedAt,
	)
	if err != nil {
		return types.Task{}, err
	}
	task.ID = types.TaskID(id)
	task.ServiceID = types.ServiceID(serviceID)
	if nodeID != nil {
		task.NodeID = types.NodeID(*nodeID)
	}
	if containerID != nil {
		task.ContainerID = *containerID
	}
	if failureReason != nil {
		task.FailureReason = *failureReason
	}
	if len(ports) > 0 {
		if err := json.Unmarshal(ports, &task.Ports); err != nil {
			return types.Task{}, err
		}
	}
	task.CreatedAt = task.CreatedAt.UTC()
	task.UpdatedAt = task.UpdatedAt.UTC()
	if startedAt != nil {
		task.StartedAt = startedAt.UTC()
	}
	if finishedAt != nil {
		task.FinishedAt = finishedAt.UTC()
	}
	return task, nil
}

func scanSecret(row pgx.Row) (types.Secret, error) {
	var secret types.Secret
	err := row.Scan(
		&secret.Name,
		&secret.EncryptedValue,
		&secret.KeyID,
		&secret.CreatedAt,
		&secret.UpdatedAt,
	)
	if err != nil {
		return types.Secret{}, err
	}
	secret.EncryptedValue = append([]byte(nil), secret.EncryptedValue...)
	secret.CreatedAt = secret.CreatedAt.UTC()
	secret.UpdatedAt = secret.UpdatedAt.UTC()
	return secret, nil
}

func scanRegistryCredential(row pgx.Row) (types.RegistryCredential, error) {
	var credential types.RegistryCredential
	err := row.Scan(
		&credential.ID,
		&credential.Registry,
		&credential.Username,
		&credential.EncryptedPassword,
		&credential.KeyID,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	)
	if err != nil {
		return types.RegistryCredential{}, err
	}
	credential.EncryptedPassword = append([]byte(nil), credential.EncryptedPassword...)
	credential.CreatedAt = credential.CreatedAt.UTC()
	credential.UpdatedAt = credential.UpdatedAt.UTC()
	return credential, nil
}

func scanDeployment(row pgx.Row) (types.Deployment, error) {
	var deployment types.Deployment
	var id, serviceID string
	var startedAt, completedAt *time.Time
	err := row.Scan(
		&id,
		&serviceID,
		&deployment.FromVersion,
		&deployment.ToVersion,
		&deployment.Strategy,
		&deployment.Status,
		&deployment.MaxUnavailable,
		&deployment.MaxSurge,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
		&startedAt,
		&completedAt,
	)
	if err != nil {
		return types.Deployment{}, err
	}
	deployment.ID = types.DeploymentID(id)
	deployment.ServiceID = types.ServiceID(serviceID)
	deployment.CreatedAt = deployment.CreatedAt.UTC()
	deployment.UpdatedAt = deployment.UpdatedAt.UTC()
	if startedAt != nil {
		deployment.StartedAt = startedAt.UTC()
	}
	if completedAt != nil {
		deployment.CompletedAt = completedAt.UTC()
	}
	return deployment, nil
}

func scanEvent(row pgx.Row) (types.Event, error) {
	var event types.Event
	var id string
	var relatedType, relatedID *string
	err := row.Scan(
		&id,
		&event.Type,
		&event.Severity,
		&event.Source,
		&event.Message,
		&relatedType,
		&relatedID,
		&event.Timestamp,
	)
	if err != nil {
		return types.Event{}, err
	}
	event.ID = types.EventID(id)
	if relatedType != nil {
		event.RelatedObjectType = *relatedType
	}
	if relatedID != nil {
		event.RelatedObjectID = *relatedID
	}
	event.Timestamp = event.Timestamp.UTC()
	return event, nil
}

func serviceJSON(spec types.ServiceSpec) ([]byte, []byte, []byte, []byte, []byte, []byte, []byte, []byte, error) {
	env, err := jsonBytes(defaultMap(spec.Env))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	secretRefs, err := jsonBytes(defaultSlice(spec.SecretRefs))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	ports, err := jsonBytes(defaultSlice(spec.Ports))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	requirements, err := jsonBytes(spec.ResourceRequirements)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	var healthcheck []byte
	if spec.Healthcheck != nil {
		healthcheck, err = jsonBytes(spec.Healthcheck)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, err
		}
	}
	restartPolicy, err := jsonBytes(spec.RestartPolicy)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	constraints, err := jsonBytes(defaultSlice(spec.PlacementConstraints))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	routes, err := jsonBytes(defaultSlice(spec.Routes))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	return env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints, routes, nil
}

func decodeServiceJSON(spec *types.ServiceSpec, env []byte, secretRefs []byte, ports []byte, requirements []byte, healthcheck []byte, restartPolicy []byte, constraints []byte, routes []byte) error {
	if err := json.Unmarshal(env, &spec.Env); err != nil {
		return fmt.Errorf("decode service env: %w", err)
	}
	if err := json.Unmarshal(secretRefs, &spec.SecretRefs); err != nil {
		return fmt.Errorf("decode service secret refs: %w", err)
	}
	if err := json.Unmarshal(ports, &spec.Ports); err != nil {
		return fmt.Errorf("decode service ports: %w", err)
	}
	if err := json.Unmarshal(requirements, &spec.ResourceRequirements); err != nil {
		return fmt.Errorf("decode service resource requirements: %w", err)
	}
	if len(healthcheck) > 0 {
		var check types.Healthcheck
		if err := json.Unmarshal(healthcheck, &check); err != nil {
			return fmt.Errorf("decode service healthcheck: %w", err)
		}
		spec.Healthcheck = &check
	}
	if err := json.Unmarshal(restartPolicy, &spec.RestartPolicy); err != nil {
		return fmt.Errorf("decode service restart policy: %w", err)
	}
	if err := json.Unmarshal(constraints, &spec.PlacementConstraints); err != nil {
		return fmt.Errorf("decode service placement constraints: %w", err)
	}
	if err := json.Unmarshal(routes, &spec.Routes); err != nil {
		return fmt.Errorf("decode service routes: %w", err)
	}
	return nil
}

func jsonBytes(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return data, nil
}

func defaultMap(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	return value
}

func defaultSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}
	return value
}

func nilTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func utcOrNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func mapPostgresError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrDuplicate, pgErr.ConstraintName)
		case "23503", "23514", "22P02":
			return fmt.Errorf("%w: %s", ErrInvalidState, pgErr.Message)
		case "40001", "40P01":
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.Message)
		}
	}
	return err
}
