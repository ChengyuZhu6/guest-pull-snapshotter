package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	guestpull "github.com/ChengyuZhu6/guest-pull-snapshotter/guest-pull"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/core/snapshots/storage"
	"github.com/containerd/continuity/fs"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/pkg/errors"
)

// Label constants for snapshotter
const (
	// targetSnapshotLabel is the label for the target snapshot reference
	targetSnapshotLabel = "containerd.io/snapshot.ref"

	// guestPullLabel indicates this is a guest pull snapshot
	guestPullLabel = "containerd.io/snapshot/guestpull"
)

// SnapshotterConfig is used to configure the remote snapshotter instance
type SnapshotterConfig struct {
	root string
}

// Opt is an option to configure the guest pull snapshotter
type Opt func(config *SnapshotterConfig)

// WithRootDirectory defines the root directory for the snapshotter
func WithRootDirectory(path string) Opt {
	return func(config *SnapshotterConfig) {
		config.root = path
	}
}

// snapshotter implements the containerd snapshotter interface
type snapshotter struct {
	root string
	ms   *storage.MetaStore
}

// NewSnapshotter creates a new snapshotter instance
func NewSnapshotter(ctx context.Context, opts ...Opt) (snapshots.Snapshotter, error) {
	var config SnapshotterConfig
	for _, opt := range opts {
		opt(&config)
	}

	if config.root == "" {
		return nil, errors.New("root directory must be specified")
	}

	snapshotsDir := filepath.Join(config.root, "snapshots")
	for _, dir := range []string{config.root, snapshotsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, errors.Wrapf(err, "failed to create directory: %s", dir)
		}
	}

	ms, err := storage.NewMetaStore(filepath.Join(config.root, "metadata.db"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create metadata store")
	}

	return &snapshotter{
		root: config.root,
		ms:   ms,
	}, nil
}

func (o *snapshotter) Close() error {
	return o.ms.Close()
}

func (o *snapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	log.G(ctx).Debugf("Prepare snapshot with key %s, parent %s, opts %v", key, parent, opts)

	var base snapshots.Info
	for _, opt := range opts {
		if err := opt(&base); err != nil {
			return nil, errors.Wrap(err, "failed to apply options")
		}
	}

	info, s, err := o.createSnapshot(ctx, snapshots.KindActive, key, parent, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create snapshot")
	}

	if info.Labels == nil {
		info.Labels = make(map[string]string)
	}

	if target, ok := info.Labels[targetSnapshotLabel]; ok {
		info.Labels[guestPullLabel] = "true"

		err := o.Commit(ctx, target, key, append(opts, snapshots.WithLabels(info.Labels))...)
		if err == nil || errdefs.IsAlreadyExists(err) {
			return nil, errors.Wrapf(errdefs.ErrAlreadyExists, "target snapshot %q", target)
		}
		return nil, nil
	}

	if !IsGuestPullMode(info.Labels) {
		return o.mountGuestPull(ctx, s, "", false)
	}

	pID, _, _, pErr := o.getSnapshotInfo(ctx, key)
	if pErr != nil {
		return nil, errors.Wrapf(pErr, "failed to get parent snapshot info, parent key=%q", parent)
	}
	return o.mountGuestPull(ctx, s, pID, true)
}

func (o *snapshotter) withTransaction(ctx context.Context, writable bool, fn func(ctx context.Context) error) error {
	ctx, t, err := o.ms.TransactionContext(ctx, writable)
	if err != nil {
		return errors.Wrap(err, "failed to start transaction")
	}

	var done bool
	defer func() {
		if !done {
			if rerr := t.Rollback(); rerr != nil {
				log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
			}
		}
	}()

	if err := fn(ctx); err != nil {
		return err
	}

	if writable {
		if err := t.Commit(); err != nil {
			return errors.Wrap(err, "failed to commit transaction")
		}
	} else {
		if err := t.Rollback(); err != nil {
			log.G(ctx).WithError(err).Warn("failed to rollback transaction")
		}
	}

	done = true
	return nil
}

func (o *snapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	log.G(ctx).Debugf("Commit snapshot with key %s to %s", key, name)

	return o.withTransaction(ctx, true, func(ctx context.Context) error {
		id, _, _, err := storage.GetInfo(ctx, key)
		if err != nil {
			return errors.Wrap(err, "failed to get snapshot info")
		}

		du, err := fs.DiskUsage(ctx, o.upperPath(id))
		if err != nil {
			return errors.Wrap(err, "failed to calculate disk usage")
		}

		if _, err = storage.CommitActive(ctx, key, name, snapshots.Usage(du), opts...); err != nil {
			return errors.Wrapf(err, "commit active snapshot %s", key)
		}

		return nil
	})
}

func (o *snapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	log.G(ctx).Debugf("Mounts for snapshot %s", key)

	id, info, _, err := o.getSnapshotInfo(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get snapshot info for %q", key)
	}

	snap, err := o.getSnapshot(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get snapshot %s", key)
	}

	if !IsGuestPullMode(info.Labels) {
		return o.mountGuestPull(ctx, *snap, "", false)
	}

	var snapshotID string

	switch info.Kind {
	case snapshots.KindView:
		snapshotID = id
	case snapshots.KindActive:
		if info.Parent != "" {
			pID, _, _, err := o.getSnapshotInfo(ctx, info.Parent)
			if err != nil {
				return nil, errors.Wrapf(err, "get parent snapshot info, parent key=%q", info.Parent)
			}
			snapshotID = pID
		}
	}

	return o.mountGuestPull(ctx, *snap, snapshotID, true)

}

func (o *snapshotter) Remove(ctx context.Context, key string) error {
	log.G(ctx).Debugf("Remove snapshot %s", key)

	return o.withTransaction(ctx, true, func(ctx context.Context) error {
		_, _, err := storage.Remove(ctx, key)
		return errors.Wrap(err, "failed to remove snapshot")
	})
}

func (o *snapshotter) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	log.G(ctx).Debugf("Stat snapshot %s", key)
	_, info, _, err := o.getSnapshotInfo(ctx, key)
	return info, err
}

func (o *snapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	log.G(ctx).Debugf("Update snapshot %s", info.Name)

	var updated snapshots.Info
	err := o.withTransaction(ctx, true, func(ctx context.Context) error {
		var err error
		updated, err = storage.UpdateInfo(ctx, info, fieldpaths...)
		return errors.Wrap(err, "failed to update info")
	})

	return updated, err
}

func (o *snapshotter) Walk(ctx context.Context, fn snapshots.WalkFunc, filters ...string) error {
	log.G(ctx).Debugf("Walk snapshots with filters %v", filters)

	return o.withTransaction(ctx, false, func(ctx context.Context) error {
		return storage.WalkInfo(ctx, fn, filters...)
	})
}

func (o *snapshotter) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	log.G(ctx).Debugf("Usage for snapshot %s", key)

	id, info, usage, err := o.getSnapshotInfo(ctx, key)
	if err != nil {
		return snapshots.Usage{}, errors.Wrap(err, "failed to get snapshot info")
	}

	if info.Kind == snapshots.KindActive {
		du, err := fs.DiskUsage(ctx, o.upperPath(id))
		if err != nil {
			return snapshots.Usage{}, errors.Wrap(err, "failed to calculate disk usage")
		}
		usage = snapshots.Usage(du)
	}

	return usage, nil
}

func (o *snapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	log.G(ctx).Debugf("View snapshot with key %s, parent %s", key, parent)

	pID, _, _, err := o.getSnapshotInfo(ctx, parent)
	if err != nil {
		return nil, errors.Wrapf(err, "get snapshot %s info", parent)
	}

	_, s, err := o.createSnapshot(ctx, snapshots.KindView, key, parent, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create view snapshot")
	}

	return o.mountGuestPull(ctx, s, pID, true)
}

func (o *snapshotter) createSnapshot(ctx context.Context, kind snapshots.Kind, key, parent string, opts []snapshots.Opt) (info *snapshots.Info, _ storage.Snapshot, err error) {
	var (
		s    storage.Snapshot
		base snapshots.Info
		td   string
		path string
	)

	for _, opt := range opts {
		if err := opt(&base); err != nil {
			return &base, storage.Snapshot{}, errors.Wrap(err, "failed to apply option")
		}
	}

	if base.Labels == nil {
		base.Labels = map[string]string{}
	}

	err = o.withTransaction(ctx, true, func(ctx context.Context) error {
		snapshotDir := filepath.Join(o.root, "snapshots")
		td, err = o.prepareDirectory(snapshotDir, kind)
		if err != nil {
			return errors.Wrap(err, "failed to prepare snapshot directory")
		}

		defer func() {
			if err != nil && td != "" {
				if err1 := o.cleanupSnapshotDirectory(td); err1 != nil {
					log.G(ctx).WithError(err1).Warn("failed to cleanup temp snapshot directory")
				}
			}
		}()

		s, err = storage.CreateSnapshot(ctx, kind, key, parent, opts...)
		if err != nil {
			return errors.Wrap(err, "failed to create snapshot in metadata store")
		}

		if err := o.setupSnapshotDirectory(td, s); err != nil {
			return errors.Wrap(err, "failed to setup snapshot directory")
		}

		path = filepath.Join(snapshotDir, s.ID)
		if err = os.Rename(td, path); err != nil {
			return errors.Wrap(err, "failed to rename snapshot directory")
		}
		td = ""

		return nil
	})

	if err != nil && path != "" {
		if err1 := o.cleanupSnapshotDirectory(path); err1 != nil {
			log.G(ctx).WithError(err1).WithField("path", path).Error("failed to reclaim snapshot directory")
			err = fmt.Errorf("failed to remove path: %v: %w", err1, err)
		}
		return &base, storage.Snapshot{}, err
	}

	return &base, s, nil
}

func (o *snapshotter) setupSnapshotDirectory(td string, s storage.Snapshot) error {
	if len(s.ParentIDs) > 0 {
		st, err := os.Stat(o.upperPath(s.ParentIDs[0]))
		if err != nil {
			if !os.IsNotExist(err) {
				return errors.Wrap(err, "failed to stat parent")
			}
			return os.MkdirAll(o.upperPath(s.ParentIDs[0]), 0755)
		}

		stat := st.Sys().(*syscall.Stat_t)
		return os.Lchown(filepath.Join(td, "fs"), int(stat.Uid), int(stat.Gid))
	}
	return nil
}

func (o *snapshotter) prepareDirectory(snapshotDir string, kind snapshots.Kind) (string, error) {
	td, err := os.MkdirTemp(snapshotDir, "new-")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temporary directory")
	}

	if err := os.Mkdir(filepath.Join(td, "fs"), 0755); err != nil {
		return td, errors.Wrap(err, "failed to create fs directory")
	}

	if kind == snapshots.KindActive {
		if err := os.Mkdir(filepath.Join(td, "work"), 0711); err != nil {
			return td, errors.Wrap(err, "failed to create work directory")
		}
	}

	return td, nil
}

func (o *snapshotter) cleanupSnapshotDirectory(dir string) error {
	return errors.Wrapf(os.RemoveAll(dir), "failed to remove directory %q", dir)
}

func (o *snapshotter) getSnapshotInfo(ctx context.Context, key string) (id string, info snapshots.Info, usage snapshots.Usage, err error) {
	err = o.withTransaction(ctx, false, func(ctx context.Context) error {
		id, info, usage, err = storage.GetInfo(ctx, key)
		return errors.Wrapf(err, "failed to get info for %s", key)
	})

	return
}

func (o *snapshotter) getSnapshot(ctx context.Context, key string) (*storage.Snapshot, error) {
	var snapshot storage.Snapshot
	err := o.withTransaction(ctx, false, func(ctx context.Context) error {
		var err error
		snapshot, err = storage.GetSnapshot(ctx, key)
		return errors.Wrapf(err, "failed to get snapshot for %s", key)
	})

	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

func (o *snapshotter) mountGuestPull(ctx context.Context, s storage.Snapshot, id string, flag bool) ([]mount.Mount, error) {
	var overlayOptions []string

	if s.Kind == snapshots.KindActive {
		overlayOptions = append(overlayOptions,
			fmt.Sprintf("workdir=%s", o.workPath(s.ID)),
			fmt.Sprintf("upperdir=%s", o.upperPath(s.ID)),
		)
	}

	var lowerPaths []string
	if flag && s.Kind == snapshots.KindView {
		if lowerPath, err := o.lowerPath(id); err == nil {
			lowerPaths = append(lowerPaths, lowerPath)
		} else {
			log.L.WithError(err).Warnf("failed to get lower path for %s", id)
		}
	} else if len(s.ParentIDs) == 0 {
		lowerPaths = append(lowerPaths, filepath.Join(o.root, "snapshots"))
	} else {
		for _, id := range s.ParentIDs {
			lowerPaths = append(lowerPaths, o.upperPath(id))
		}
	}
	
	overlayOptions = append(overlayOptions, fmt.Sprintf("lowerdir=%s", strings.Join(lowerPaths, ":")))

	guestOptions, err := guestpull.PrepareGuestPullMounts(ctx, "", overlayOptions, map[string]string{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare guest pull mounts for snapshot %s", s.ID)
	}

	if len(guestOptions) > 0 {
		overlayOptions = append(overlayOptions, guestOptions...)
	}

	return []mount.Mount{
		{
			Type:    "fuse.guest-pull-overlayfs",
			Source:  "overlay",
			Options: overlayOptions,
		},
	}, nil
}

func (o *snapshotter) upperPath(id string) string {
	return filepath.Join(o.root, "snapshots", id, "fs")
}

func (o *snapshotter) workPath(id string) string {
	return filepath.Join(o.root, "snapshots", id, "work")
}

func (o *snapshotter) lowerPath(id string) (string, error) {
	return filepath.Join(o.root, "snapshots", id, "fs"), nil
}

func IsGuestPullMode(labels map[string]string) bool {
	_, ok := labels[guestPullLabel]
	return ok
}
