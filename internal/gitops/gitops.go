package gitops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/pkg/types"
)

const (
	EventSyncSucceeded = "gitops.sync.succeeded"
	EventSyncFailed    = "gitops.sync.failed"
	EventDriftDetected = "gitops.drift.detected"
	EventDriftReverted = "gitops.drift.reverted"
)

type Diff struct {
	Service string                  `json:"service"`
	Status  types.GitOpsDriftStatus `json:"status"`
	Desired types.ServiceSpec       `json:"desired"`
	Live    types.ServiceSpec       `json:"live"`
}

type managedStore interface {
	MarkGitOpsManaged(context.Context, types.ServiceID, types.GitOpsManagedState) (types.Service, error)
}

type Store interface {
	ListNamespaces(context.Context) ([]types.Namespace, error)
	ListGitOpsSources(context.Context) ([]types.GitOpsSource, error)
	GetGitOpsSource(context.Context, string) (types.GitOpsSource, error)
	UpdateGitOpsSource(context.Context, types.GitOpsSource) (types.GitOpsSource, error)
	ApplyService(context.Context, types.ServiceSpec) (types.Service, error)
	ListServices(context.Context) ([]types.Service, error)
	DeleteService(context.Context, types.ServiceID) error
	AppendEvent(context.Context, types.Event) (types.Event, error)
	AppendAuditLog(context.Context, audit.Log) (audit.Log, error)
}

type Parser func([]byte) (types.ServiceSpec, error)

type Checkout struct {
	Directory string
	Revision  string
	Cleanup   func()
}

type Fetcher interface {
	Checkout(context.Context, types.GitOpsSource) (Checkout, error)
}

type CommandFetcher struct{}

func (CommandFetcher) Checkout(ctx context.Context, source types.GitOpsSource) (Checkout, error) {
	if err := ValidateSource(source); err != nil {
		return Checkout{}, err
	}
	directory, err := os.MkdirTemp("", "orch-gitops-")
	if err != nil {
		return Checkout{}, fmt.Errorf("create checkout directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	args := []string{"clone", "--depth=1", "--single-branch", "--branch", source.Branch, "--", source.RepositoryURL, directory}
	command := exec.CommandContext(ctx, "git", args...)
	if output, err := command.CombinedOutput(); err != nil {
		cleanup()
		return Checkout{}, fmt.Errorf("clone repository: %w: %s", err, strings.TrimSpace(string(output)))
	}
	revisionBytes, err := exec.CommandContext(ctx, "git", "-C", directory, "rev-parse", "HEAD").Output()
	if err != nil {
		cleanup()
		return Checkout{}, fmt.Errorf("resolve checkout revision: %w", err)
	}
	return Checkout{Directory: directory, Revision: strings.TrimSpace(string(revisionBytes)), Cleanup: cleanup}, nil
}

func ValidateSource(source types.GitOpsSource) error {
	if strings.TrimSpace(source.RepositoryURL) == "" {
		return fmt.Errorf("repository_url is required")
	}
	if strings.TrimSpace(source.Branch) == "" {
		return fmt.Errorf("branch is required")
	}
	if source.SyncInterval <= 0 {
		return fmt.Errorf("sync_interval must be positive")
	}
	if source.DriftPolicy != "" && source.DriftPolicy != types.GitOpsWarnOnly && source.DriftPolicy != types.GitOpsAutoRevert {
		return fmt.Errorf("drift_policy must be warn or auto_revert")
	}
	parsed, err := url.Parse(source.RepositoryURL)
	if err != nil {
		return fmt.Errorf("repository_url is invalid: %w", err)
	}
	if parsed.User != nil {
		return fmt.Errorf("repository_url must not contain credentials")
	}
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "key") {
			return fmt.Errorf("repository_url must not contain credential query parameters")
		}
	}
	cleanPath := filepath.Clean(strings.TrimSpace(source.Path))
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must stay within the repository")
	}
	return nil
}

type Controller struct {
	store      Store
	fetcher    Fetcher
	parser     Parser
	logger     *slog.Logger
	resolution time.Duration
}

type Option func(*Controller)

func WithResolution(value time.Duration) Option {
	return func(controller *Controller) {
		if value > 0 {
			controller.resolution = value
		}
	}
}

func NewController(store Store, fetcher Fetcher, parser Parser, logger *slog.Logger, opts ...Option) *Controller {
	if fetcher == nil {
		fetcher = CommandFetcher{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	controller := &Controller{store: store, fetcher: fetcher, parser: parser, logger: logger, resolution: time.Second}
	for _, opt := range opts {
		opt(controller)
	}
	return controller
}

func (controller *Controller) Run(ctx context.Context) error {
	if controller == nil || controller.store == nil || controller.parser == nil {
		return fmt.Errorf("gitops controller is not configured")
	}
	ticker := time.NewTicker(controller.resolution)
	defer ticker.Stop()
	if err := controller.syncDue(ctx); err != nil && !errors.Is(err, context.Canceled) {
		controller.logger.Warn("GitOps sync iteration failed", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := controller.syncDue(ctx); err != nil && !errors.Is(err, context.Canceled) {
				controller.logger.Warn("GitOps sync iteration failed", "error", err)
			}
		}
	}
}

func (controller *Controller) syncDue(ctx context.Context) error {
	namespaces, err := controller.store.ListNamespaces(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, item := range namespaces {
		namespaceCtx := namespace.WithContext(ctx, item.Name)
		sources, err := controller.store.ListGitOpsSources(namespaceCtx)
		if err != nil {
			return err
		}
		for _, source := range sources {
			if !source.LastSyncedAt.IsZero() && now.Before(source.LastSyncedAt.Add(source.SyncInterval)) {
				continue
			}
			if _, err := controller.Sync(namespaceCtx, source.ID); err != nil {
				controller.logger.Warn("GitOps source sync failed", "source_id", source.ID, "namespace", source.Namespace, "error", err)
			}
		}
	}
	return nil
}

func (controller *Controller) Sync(ctx context.Context, id string) (types.GitOpsSource, error) {
	source, err := controller.store.GetGitOpsSource(ctx, id)
	if err != nil {
		return types.GitOpsSource{}, err
	}
	if err := ValidateSource(source); err != nil {
		return controller.fail(ctx, source, err)
	}
	checkout, err := controller.fetcher.Checkout(ctx, source)
	if err != nil {
		return controller.fail(ctx, source, err)
	}
	if checkout.Cleanup != nil {
		defer checkout.Cleanup()
	}
	files, err := manifestFiles(checkout.Directory, source.Path)
	if err != nil {
		return controller.fail(ctx, source, err)
	}

	syncCtx := namespace.WithContext(ctx, source.Namespace)
	managed := make([]string, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return controller.fail(syncCtx, source, fmt.Errorf("read manifest %q: %w", file, err))
		}
		spec, err := controller.parser(data)
		if err != nil {
			return controller.fail(syncCtx, source, fmt.Errorf("validate manifest %q: %w", file, err))
		}
		service, err := controller.store.ApplyService(syncCtx, spec)
		if err != nil {
			return controller.fail(syncCtx, source, fmt.Errorf("apply service %q: %w", spec.Name, err))
		}
		if marker, ok := controller.store.(managedStore); ok {
			relative, _ := filepath.Rel(checkout.Directory, file)
			policy := source.DriftPolicy
			if policy == "" {
				policy = types.GitOpsWarnOnly
			}
			if _, err := marker.MarkGitOpsManaged(syncCtx, service.ID, types.GitOpsManagedState{SourceID: source.ID, SourceCommit: checkout.Revision, SourcePath: filepath.ToSlash(relative), Status: types.GitOpsInSync, Policy: policy, DesiredSpec: spec, LastCheckedAt: time.Now().UTC()}); err != nil {
				return controller.fail(syncCtx, source, fmt.Errorf("mark service %q managed: %w", spec.Name, err))
			}
		}
		managed = append(managed, spec.Name)
	}
	slices.Sort(managed)
	if source.Prune {
		wanted := make(map[string]struct{}, len(managed))
		for _, name := range managed {
			wanted[name] = struct{}{}
		}
		previous := make(map[string]struct{}, len(source.ManagedServices))
		for _, name := range source.ManagedServices {
			previous[name] = struct{}{}
		}
		services, err := controller.store.ListServices(syncCtx)
		if err != nil {
			return controller.fail(syncCtx, source, err)
		}
		for _, service := range services {
			if _, wasManaged := previous[service.Spec.Name]; !wasManaged {
				continue
			}
			if _, stillManaged := wanted[service.Spec.Name]; stillManaged {
				continue
			}
			if err := controller.store.DeleteService(syncCtx, service.ID); err != nil {
				return controller.fail(syncCtx, source, fmt.Errorf("prune service %q: %w", service.Spec.Name, err))
			}
		}
	}

	source.ManagedServices = managed
	source.LastRevision = checkout.Revision
	source.LastError = ""
	source.LastSyncedAt = time.Now().UTC()
	updated, err := controller.store.UpdateGitOpsSource(syncCtx, source)
	if err != nil {
		return types.GitOpsSource{}, err
	}
	controller.emit(syncCtx, updated, EventSyncSucceeded, types.EventInfo, audit.OutcomeSuccess, "GitOps source synchronized")
	return updated, nil
}

func (controller *Controller) fail(ctx context.Context, source types.GitOpsSource, cause error) (types.GitOpsSource, error) {
	source.LastError = cause.Error()
	source.LastSyncedAt = time.Now().UTC()
	updated, updateErr := controller.store.UpdateGitOpsSource(namespace.WithContext(ctx, source.Namespace), source)
	if updateErr == nil {
		controller.emit(namespace.WithContext(ctx, source.Namespace), updated, EventSyncFailed, types.EventError, audit.OutcomeFailure, cause.Error())
	}
	return updated, cause
}

func (controller *Controller) emit(ctx context.Context, source types.GitOpsSource, eventType string, severity types.EventSeverity, outcome audit.Outcome, message string) {
	now := time.Now().UTC()
	_, _ = controller.store.AppendEvent(ctx, types.Event{Namespace: source.Namespace, Type: eventType, Severity: severity, Source: "gitops", Message: message, RelatedObjectType: "gitops_source", RelatedObjectID: source.ID, Timestamp: now})
	_, _ = controller.store.AppendAuditLog(ctx, audit.Log{Namespace: source.Namespace, ActorType: audit.ActorSystem, ActorID: "gitops-controller", Action: "gitops.sync", TargetType: "gitops_source", TargetID: source.ID, Outcome: outcome, Metadata: map[string]string{"revision": source.LastRevision}, Timestamp: now})
}

func manifestFiles(root string, configuredPath string) ([]string, error) {
	base := filepath.Join(root, filepath.Clean(configuredPath))
	relative, err := filepath.Rel(root, base)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("configured path escapes repository")
	}
	info, err := os.Stat(base)
	if err != nil {
		return nil, fmt.Errorf("stat configured path: %w", err)
	}
	if !info.IsDir() {
		if isYAML(base) {
			return []string{base}, nil
		}
		return nil, fmt.Errorf("configured manifest is not YAML")
	}
	files := make([]string, 0)
	err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && isYAML(path) {
			files = append(files, path)
		}
		return nil
	})
	slices.Sort(files)
	if err != nil {
		return nil, err
	}
	return files, nil
}

func isYAML(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".yaml" || extension == ".yml"
}
