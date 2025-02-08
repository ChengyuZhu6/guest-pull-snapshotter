package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/storage"
	"github.com/containerd/log"
)

// SnapshotterConfig is used to configure the remote snapshotter instance
type SnapshotterConfig struct {
	root                   string
	guestPullOverlayFSPath string
}

// WithGuestPullOverlayFSPath defines the path to the guest pull overlayfs
func WithGuestPullOverlayFSPath(path string) Opt {
	return func(config *SnapshotterConfig) {
		config.guestPullOverlayFSPath = path
	}
}

// Opt is an option to configure the guest pull snapshotter
type Opt func(config *SnapshotterConfig)

type snapshotter struct {
	root                   string
	ms                     *storage.MetaStore
	guestPullOverlayFSPath string
}

func NewSnapshotter(ctx context.Context, root string, opts ...Opt) (snapshots.Snapshotter, error) {
	var config SnapshotterConfig
	for _, opt := range opts {
		opt(&config)
	}

	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}

	ms, err := storage.NewMetaStore(filepath.Join(root, "metadata.db"))
	if err != nil {
		return nil, err
	}

	if err := os.Mkdir(filepath.Join(root, "snapshots"), 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}

	return &snapshotter{
		root:                   root,
		ms:                     ms,
		guestPullOverlayFSPath: config.guestPullOverlayFSPath,
	}, nil
}

// Close closes the snapshotter
func (o *snapshotter) Close() error {
	return o.ms.Close()
}

func (o *snapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	log.L.Debugf("[Prepare] snapshot with key %s", key)
	return nil, nil
}

// Commit commits a new snapshot
func (o *snapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	log.L.Debugf("[Commit] snapshot with key %s", key)
	return nil
}
func (o *snapshotter) Mounts(ctx context.Context, key string) (_ []mount.Mount, err error) {
	log.L.Debugf("[Mounts] snapshot with key %s", key)
	return []mount.Mount{{}}, nil
}

func (o *snapshotter) Remove(ctx context.Context, key string) error {
	log.L.Debugf("[Remove] snapshot with key %s", key)
	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rerr := t.Rollback(); rerr != nil {
				log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
			}
		}
	}()

	_, _, err = storage.Remove(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to remove: %w", err)
	}
	return t.Commit()
}

func (o *snapshotter) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	log.L.Debugf("[Stat] snapshot with key %s", key)
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return snapshots.Info{}, err
	}
	defer t.Rollback()
	_, info, _, err := storage.GetInfo(ctx, key)
	if err != nil {
		return snapshots.Info{}, err
	}

	return info, nil
}

func (o *snapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	log.L.Debugf("[Update] snapshot with key %s", info.Name)
	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return snapshots.Info{}, err
	}

	info, err = storage.UpdateInfo(ctx, info, fieldpaths...)
	if err != nil {
		t.Rollback()
		return snapshots.Info{}, err
	}

	if err := t.Commit(); err != nil {
		return snapshots.Info{}, err
	}

	return info, nil
}

func (o *snapshotter) Walk(ctx context.Context, fn snapshots.WalkFunc, fs ...string) error {
	log.L.Debugf("[Walk] snapshot with key %s", fs)
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return err
	}
	defer func() {
		if err := t.Rollback(); err != nil {
			log.L.WithError(err)
		}
	}()

	return storage.WalkInfo(ctx, fn, fs...)
}

func (o *snapshotter) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	log.L.Debugf("[Usage] snapshot with key %s", key)
	return snapshots.Usage{}, nil
}

func (o *snapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	log.L.Debugf("[View] snapshot with key %s, parent %s", key, parent)
	return []mount.Mount{{}}, nil
}
