package snapshots

import (
	"context"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/storage"
	"github.com/sirupsen/logrus"
)

type snapshotter struct {
	root string
	ms   *storage.MetaStore
	log  *logrus.Logger
}

// New creates a new remote snapshotter
func New(root string) (snapshots.Snapshotter, error) {
	ms, err := storage.NewMetaStore(root)
	if err != nil {
		return nil, err
	}

	return &snapshotter{
		root: root,
		ms:   ms,
		log:  logrus.New(),
	}, nil
}

// Stat returns the info for a snapshot
func (s *snapshotter) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	_, info, _, err := storage.GetInfo(ctx, key)
	return info, err
}

// Update updates the info for a snapshot
func (s *snapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	return storage.UpdateInfo(ctx, info, fieldpaths...)
}

// Usage returns the resources taken by a snapshot
func (s *snapshotter) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	_, _, usage, err := storage.GetInfo(ctx, key)
	return usage, err
}

// Prepare creates a new snapshot identified by key
func (s *snapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	// Implementation for preparing a new snapshot
	return []mount.Mount{}, nil
}

// View creates a read-only snapshot
func (s *snapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	return []mount.Mount{}, nil
}

// Commit commits a snapshot
func (s *snapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	return nil
}

// Remove removes a snapshot
func (s *snapshotter) Remove(ctx context.Context, key string) error {
	_, _, err := storage.Remove(ctx, key)
	return err
}

// Walk walks all snapshots
func (s *snapshotter) Walk(ctx context.Context, fn snapshots.WalkFunc, filters ...string) error {
	return storage.WalkInfo(ctx, fn, filters...)
}

// Mounts returns the mounts for the transaction identified by key
func (s *snapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	return nil, nil
}

// Close closes the snapshotter
func (s *snapshotter) Close() error {
	return s.ms.Close()
}
