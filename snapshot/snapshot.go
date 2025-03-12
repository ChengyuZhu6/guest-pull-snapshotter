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

	if err := os.MkdirAll(config.root, 0700); err != nil {
		return nil, errors.Wrap(err, "failed to create root directory")
	}

	ms, err := storage.NewMetaStore(filepath.Join(config.root, "metadata.db"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create metadata store")
	}

	snapshotsDir := filepath.Join(config.root, "snapshots")
	if err := os.Mkdir(snapshotsDir, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.Wrap(err, "failed to create snapshots directory")
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
	log.G(ctx).Infof("Prepare snapshot with key %s, parent %s", key, parent)

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

	// Initialize Labels if nil
	if info.Labels == nil {
		info.Labels = make(map[string]string)
	}

	// Add guest-pull label
	info.Labels[guestPullLabel] = "true"

	if len(s.ParentIDs) == 0 {
		roFlag := "rw"
		if s.Kind == snapshots.KindView {
			roFlag = "ro"
		}
		return bindMount(o.upperPath(s.ID), roFlag), nil
	}

	// Handle target snapshot case (read-only layer)
	if target, ok := info.Labels[targetSnapshotLabel]; ok {
		err := o.Commit(ctx, target, key, append(opts, snapshots.WithLabels(info.Labels))...)
		if err == nil || errdefs.IsAlreadyExists(err) {
			return nil, errors.Wrapf(errdefs.ErrAlreadyExists, "target snapshot %q", target)
		}
		return nil, nil
	}

	return o.mountWithGuestPull(ctx, s)
}

func (o *snapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	log.G(ctx).Infof("Commit snapshot with key %s to %s", key, name)

	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return errors.Wrap(err, "failed to start transaction")
	}

	rollback := true
	defer func() {
		if rollback {
			if rerr := t.Rollback(); rerr != nil {
				log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
			}
		}
	}()

	id, _, _, err := storage.GetInfo(ctx, key)
	if err != nil {
		return errors.Wrap(err, "failed to get snapshot info")
	}

	du, err := fs.DiskUsage(ctx, o.upperPath(id))
	if err != nil {
		return errors.Wrap(err, "failed to calculate disk usage")
	}
	usage := snapshots.Usage(du)

	if _, err = storage.CommitActive(ctx, key, name, usage, opts...); err != nil {
		return errors.Wrap(err, "failed to commit snapshot")
	}

	rollback = false
	return t.Commit()
}

func (o *snapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	log.G(ctx).Infof("Mounts for snapshot %s", key)

	id, info, _, err := o.getSnapshotInfo(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get snapshot info for %q", key)
	}
	log.G(ctx).Debugf("Mounts snapshot %s ID %s Kind %s", key, id, info.Kind)

	snap, err := o.getSnapshot(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get snapshot %s", key)
	}

	return o.mountWithGuestPull(ctx, *snap)
}

func (o *snapshotter) Remove(ctx context.Context, key string) error {
	log.G(ctx).Infof("Remove snapshot %s", key)

	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return errors.Wrap(err, "failed to start transaction")
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
		return errors.Wrap(err, "failed to remove snapshot")
	}

	return t.Commit()
}

func (o *snapshotter) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	log.G(ctx).Infof("Stat snapshot %s", key)
	_, info, _, err := o.getSnapshotInfo(ctx, key)
	return info, err
}

func (o *snapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	log.G(ctx).Infof("Update snapshot %s", info.Name)

	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return snapshots.Info{}, errors.Wrap(err, "failed to start transaction")
	}

	info, err = storage.UpdateInfo(ctx, info, fieldpaths...)
	if err != nil {
		if rerr := t.Rollback(); rerr != nil {
			log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
		}
		return snapshots.Info{}, errors.Wrap(err, "failed to update info")
	}

	if err := t.Commit(); err != nil {
		return snapshots.Info{}, errors.Wrap(err, "failed to commit transaction")
	}

	return info, nil
}

func (o *snapshotter) Walk(ctx context.Context, fn snapshots.WalkFunc, filters ...string) error {
	log.G(ctx).Infof("Walk snapshots with filters %v", filters)

	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return errors.Wrap(err, "failed to start transaction")
	}

	defer func() {
		if err := t.Rollback(); err != nil {
			log.G(ctx).WithError(err).Warn("failed to rollback transaction")
		}
	}()

	return storage.WalkInfo(ctx, fn, filters...)
}

func (o *snapshotter) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	log.G(ctx).Infof("Usage for snapshot %s", key)

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
	log.G(ctx).Infof("View snapshot with key %s, parent %s", key, parent)

	_, s, err := o.createSnapshot(ctx, snapshots.KindView, key, parent, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create view snapshot")
	}

	return o.mountWithGuestPull(ctx, s)
}

func (o *snapshotter) createSnapshot(ctx context.Context, kind snapshots.Kind, key, parent string, opts []snapshots.Opt) (info *snapshots.Info, _ storage.Snapshot, err error) {
	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return nil, storage.Snapshot{}, errors.Wrap(err, "failed to start transaction")
	}

	rollback := true
	defer func() {
		if rollback {
			if rerr := t.Rollback(); rerr != nil {
				log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
			}
		}
	}()

	var base snapshots.Info
	for _, opt := range opts {
		if err := opt(&base); err != nil {
			return &base, storage.Snapshot{}, errors.Wrap(err, "failed to apply option")
		}
	}

	if base.Labels == nil {
		base.Labels = map[string]string{}
	}

	var td, path string
	defer func() {
		if err != nil {
			if td != "" {
				if err1 := o.cleanupSnapshotDirectory(td); err1 != nil {
					log.G(ctx).WithError(err1).Warn("failed to cleanup temp snapshot directory")
				}
			}
			if path != "" {
				if err1 := o.cleanupSnapshotDirectory(path); err1 != nil {
					log.G(ctx).WithError(err1).WithField("path", path).Error("failed to reclaim snapshot directory")
					err = fmt.Errorf("failed to remove path: %v: %w", err1, err)
				}
			}
		}
	}()

	// Create temporary directory for the snapshot
	snapshotDir := filepath.Join(o.root, "snapshots")
	td, err = o.prepareDirectory(snapshotDir, kind)
	if err != nil {
		return nil, storage.Snapshot{}, errors.Wrap(err, "failed to prepare snapshot directory")
	}

	// Create the snapshot in the metadata store
	s, err := storage.CreateSnapshot(ctx, kind, key, parent, opts...)
	if err != nil {
		return nil, storage.Snapshot{}, errors.Wrap(err, "failed to create snapshot in metadata store")
	}

	// Set up the snapshot directory structure
	if err := o.setupSnapshotDirectory(td, s, parent); err != nil {
		return nil, storage.Snapshot{}, errors.Wrap(err, "failed to setup snapshot directory")
	}

	// Rename the temporary directory to the final path
	path = filepath.Join(snapshotDir, s.ID)
	if err = os.Rename(td, path); err != nil {
		return nil, storage.Snapshot{}, errors.Wrap(err, "failed to rename snapshot directory")
	}
	td = ""

	rollback = false
	if err = t.Commit(); err != nil {
		return nil, storage.Snapshot{}, errors.Wrap(err, "failed to commit transaction")
	}

	return &base, s, nil
}

func (o *snapshotter) setupSnapshotDirectory(td string, s storage.Snapshot, parent string) error {
	if len(s.ParentIDs) > 0 {
		st, err := os.Stat(o.upperPath(s.ParentIDs[0]))
		if err != nil {
			if !os.IsNotExist(err) {
				return errors.Wrap(err, "failed to stat parent")
			}
			if err := os.MkdirAll(o.upperPath(s.ParentIDs[0]), 0755); err != nil {
				return errors.Wrap(err, "failed to create parent directory")
			}
		} else {
			stat := st.Sys().(*syscall.Stat_t)
			if err := os.Lchown(filepath.Join(td, "fs"), int(stat.Uid), int(stat.Gid)); err != nil {
				return errors.Wrap(err, "failed to change ownership")
			}
		}
	}
	return nil
}

// prepareDirectory creates the directory structure for a new snapshot
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
	if err := os.RemoveAll(dir); err != nil {
		return errors.Wrapf(err, "failed to remove directory %q", dir)
	}
	return nil
}

func (o *snapshotter) getSnapshotInfo(ctx context.Context, key string) (string, snapshots.Info, snapshots.Usage, error) {
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return "", snapshots.Info{}, snapshots.Usage{}, errors.Wrap(err, "failed to start transaction")
	}

	defer func() {
		if err := t.Rollback(); err != nil {
			log.G(ctx).WithError(err).Errorf("failed to rollback transaction for %s", key)
		}
	}()

	id, info, usage, err := storage.GetInfo(ctx, key)
	if err != nil {
		return "", snapshots.Info{}, snapshots.Usage{}, errors.Wrapf(err, "failed to get info for %s", key)
	}

	return id, info, usage, nil
}

func (o *snapshotter) getSnapshot(ctx context.Context, key string) (*storage.Snapshot, error) {
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start transaction")
	}

	defer func() {
		if err := t.Rollback(); err != nil {
			log.G(ctx).WithError(err).Errorf("failed to rollback transaction for %s", key)
		}
	}()

	s, err := storage.GetSnapshot(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get snapshot for %s", key)
	}

	return &s, nil
}

// mountWithGuestPull creates mount options with guest pull support
func (o *snapshotter) mountWithGuestPull(ctx context.Context, s storage.Snapshot) ([]mount.Mount, error) {
	var overlayOptions []string

	log.G(ctx).Debugf("Mounting snapshot kind: %v", s.Kind)

	if s.Kind == snapshots.KindActive {
		overlayOptions = append(overlayOptions,
			fmt.Sprintf("workdir=%s", o.workPath(s.ID)),
			fmt.Sprintf("upperdir=%s", o.upperPath(s.ID)),
		)
	} else if len(s.ParentIDs) == 1 {
		return bindMount(o.upperPath(s.ID), "ro"), nil
	}

	parentPaths := make([]string, len(s.ParentIDs))
	for i := range s.ParentIDs {
		parentPaths[i] = o.upperPath(s.ParentIDs[i])
	}
	overlayOptions = append(overlayOptions, fmt.Sprintf("lowerdir=%s", strings.Join(parentPaths, ":")))

	log.G(ctx).Debugf("Overlay mount options: %v", overlayOptions)

	// Prepare guest pull mount options
	opt, err := guestpull.PrepareGuestPullMounts(ctx, "", overlayOptions, map[string]string{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to prepare guest pull mounts")
	}
	overlayOptions = append(overlayOptions, opt...)

	mounts := []mount.Mount{
		{
			Type:    "fuse.guest-pull-overlayfs",
			Source:  "overlay",
			Options: overlayOptions,
		},
	}
	return mounts, nil
}

func (o *snapshotter) upperPath(id string) string {
	return filepath.Join(o.root, "snapshots", id, "fs")
}

func (o *snapshotter) workPath(id string) string {
	return filepath.Join(o.root, "snapshots", id, "work")
}

func bindMount(source, roFlag string) []mount.Mount {
	return []mount.Mount{
		{
			Type:   "bind",
			Source: source,
			Options: []string{
				roFlag,
				"rbind",
			},
		},
	}
}
