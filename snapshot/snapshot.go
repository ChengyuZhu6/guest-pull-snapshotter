package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

const (
	targetSnapshotLabel    = "containerd.io/snapshot.ref"
	guestPullLabel         = "containerd.io/snapshot/guestpull"
	targetLayerDigestLabel = "containerd.io/snapshot/cri.layer-digest"
	targetRefLabel         = "containerd.io/snapshot/cri.image-ref"
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
	log.L.Infof("[Prepare] snapshot with key %s, parent %s", key, parent)

	// Log the incoming options
	var base snapshots.Info
	for _, opt := range opts {
		if err := opt(&base); err != nil {
			return nil, err
		}
	}
	log.L.Infof("Incoming options labels: %v", base.Labels)

	info, s, err := o.createSnapshot(ctx, snapshots.KindActive, key, parent, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create snapshot")
	}

	log.L.Infof("[Prepare] snapshot with key %s, parent %s, info %v, s %v, err %v", key, parent, info, s, err)

	// Initialize Labels if nil
	if info.Labels == nil {
		info.Labels = make(map[string]string)
	}

	// Add guest-pull label
	info.Labels[guestPullLabel] = "true"

	// Log all labels with their keys for better debugging
	for k, v := range info.Labels {
		log.L.Infof("Label %s: %s", k, v)
	}

	// Check for specific labels
	if ref, ok := info.Labels[targetRefLabel]; ok {
		log.L.Infof("Image reference found: %s", ref)
	} else {
		log.L.Infof("No image reference label found (%s)", targetRefLabel)
	}

	if digest, ok := info.Labels[targetLayerDigestLabel]; ok {
		log.L.Infof("Layer digest found: %s", digest)
	}

	if target, ok := info.Labels[targetSnapshotLabel]; ok {
		//ro layer
		err := o.Commit(ctx, target, key, append(opts, snapshots.WithLabels(info.Labels))...)
		if err == nil || errdefs.IsAlreadyExists(err) {
			return nil, errors.Wrapf(errdefs.ErrAlreadyExists, "target snapshot %q", target)
		}
		return nil, nil
	} else {
		//rw layer
		return o.mountWithGuestPull(ctx, s)
	}
}

// Commit commits a new snapshot
func (o *snapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	log.L.Infof("[Commit] snapshot with key %s", key)
	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return err
	}

	rollback := true
	defer func() {
		if rollback {
			if rerr := t.Rollback(); rerr != nil {
				log.G(ctx).WithError(rerr).Warn("failed to rollback transaction")
			}
		}
	}()

	// grab the existing id
	id, _, _, err := storage.GetInfo(ctx, key)
	if err != nil {
		return err
	}

	du, err := fs.DiskUsage(ctx, o.upperPath(id))
	if err != nil {
		return err
	}
	usage := snapshots.Usage(du)

	if _, err = storage.CommitActive(ctx, key, name, usage, opts...); err != nil {
		return fmt.Errorf("failed to commit snapshot: %w", err)
	}

	rollback = false
	return t.Commit()
}

func (o *snapshotter) Mounts(ctx context.Context, key string) (_ []mount.Mount, err error) {
	log.L.Infof("[Mounts] snapshot with key %s", key)
	id, info, _, err := o.getSnapshotInfo(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "mounts get snapshot %q info", key)
	}
	log.L.Infof("[Mounts] snapshot %s ID %s Kind %s", key, id, info.Kind)
	snap, err := o.getSnapshot(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "get snapshot %s", key)
	}
	return o.mountWithGuestPull(ctx, *snap)
}

func (o *snapshotter) Remove(ctx context.Context, key string) error {
	log.L.Infof("[Remove] snapshot with key %s", key)
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
	log.L.Infof("[Stat] snapshot with key %s", key)
	_, info, _, err := o.getSnapshotInfo(ctx, key)
	return info, err
}

func (o *snapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	log.L.Infof("[Update] snapshot with key %s", info.Name)
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
	log.L.Infof("[Walk] snapshot with key %s", fs)
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
	log.L.Infof("[Usage] snapshot with key %s", key)
	id, info, usage, err := o.getSnapshotInfo(ctx, key)
	if err != nil {
		return snapshots.Usage{}, err
	}

	upperPath := o.upperPath(id)

	if info.Kind == snapshots.KindActive {
		du, err := fs.DiskUsage(ctx, upperPath)
		if err != nil {
			return snapshots.Usage{}, err
		}

		usage = snapshots.Usage(du)
	}

	return usage, nil
}

func (o *snapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	log.L.Infof("[View] snapshot with key %s, parent %s", key, parent)
	_, s, err := o.createSnapshot(ctx, snapshots.KindView, key, parent, opts)
	if err != nil {
		return nil, err
	}
	return o.mountWithGuestPull(ctx, s)
}

func (o *snapshotter) createSnapshot(ctx context.Context, kind snapshots.Kind, key, parent string, opts []snapshots.Opt) (info *snapshots.Info, _ storage.Snapshot, err error) {
	ctx, t, err := o.ms.TransactionContext(ctx, true)
	if err != nil {
		return nil, storage.Snapshot{}, err
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
			return &base, storage.Snapshot{}, err
		}
	}

	log.L.Infof("[createSnapshot] base info: %+v", base)
	log.L.Infof("[createSnapshot] base labels: %v", base.Labels)

	if base.Labels == nil {
		base.Labels = map[string]string{}
	}

	var td, path string
	defer func() {
		if err != nil {
			if td != "" {
				if err1 := o.cleanupSnapshotDirectory(ctx, td); err1 != nil {
					log.G(ctx).WithError(err1).Warn("failed to cleanup temp snapshot directory")
				}
			}
			if path != "" {
				if err1 := o.cleanupSnapshotDirectory(ctx, path); err1 != nil {
					log.G(ctx).WithError(err1).WithField("path", path).Error("failed to reclaim snapshot directory, directory may need removal")
					err = fmt.Errorf("failed to remove path: %v: %w", err1, err)
				}
			}
		}
	}()

	snapshotDir := filepath.Join(o.root, "snapshots")
	td, err = o.prepareDirectory(ctx, snapshotDir, kind)
	if err != nil {
		return nil, storage.Snapshot{}, fmt.Errorf("failed to create prepare snapshot dir: %w", err)
	}

	s, err := storage.CreateSnapshot(ctx, kind, key, parent, opts...)
	if err != nil {
		return nil, storage.Snapshot{}, fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Set up the snapshot directory structure
	if err := o.setupSnapshotDirectory(ctx, td, s, parent); err != nil {
		return nil, storage.Snapshot{}, err
	}

	path = filepath.Join(snapshotDir, s.ID)
	if err = os.Rename(td, path); err != nil {
		return nil, storage.Snapshot{}, fmt.Errorf("failed to rename: %w", err)
	}
	td = ""

	rollback = false
	if err = t.Commit(); err != nil {
		return nil, storage.Snapshot{}, fmt.Errorf("commit failed: %w", err)
	}

	return &base, s, nil
}

func (o *snapshotter) setupSnapshotDirectory(ctx context.Context, td string, s storage.Snapshot, parent string) error {
	if len(s.ParentIDs) > 0 {
		st, err := os.Stat(o.upperPath(s.ParentIDs[0]))
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to stat parent: %w", err)
			}
			// If parent path doesn't exist, create it
			if err := os.MkdirAll(o.upperPath(s.ParentIDs[0]), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}
		} else {
			// If parent exists, copy permissions
			stat := st.Sys().(*syscall.Stat_t)
			if err := os.Lchown(filepath.Join(td, "fs"), int(stat.Uid), int(stat.Gid)); err != nil {
				return fmt.Errorf("failed to chown: %w", err)
			}
		}
	}

	// Create low directory for overlayfs
	if err := os.MkdirAll(filepath.Join(td, "low"), 0755); err != nil {
		return fmt.Errorf("failed to create low directory: %w", err)
	}

	return nil
}

func (o *snapshotter) prepareDirectory(ctx context.Context, snapshotDir string, kind snapshots.Kind) (string, error) {
	td, err := os.MkdirTemp(snapshotDir, "new-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	if err := os.Mkdir(filepath.Join(td, "fs"), 0755); err != nil {
		return td, err
	}

	if kind == snapshots.KindActive {
		if err := os.Mkdir(filepath.Join(td, "work"), 0711); err != nil {
			return td, err
		}
	}

	return td, nil
}

func (o *snapshotter) cleanupSnapshotDirectory(ctx context.Context, dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove directory %q: %w", dir, err)
	}
	return nil
}

func (o *snapshotter) getSnapshotInfo(ctx context.Context, key string) (string, snapshots.Info, snapshots.Usage, error) {
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return "", snapshots.Info{}, snapshots.Usage{}, err
	}

	defer func() {
		if err := t.Rollback(); err != nil {
			log.L.WithError(err).Errorf("Rollback traction %s", key)
		}
	}()

	id, info, usage, err := storage.GetInfo(ctx, key)
	if err != nil {
		return "", snapshots.Info{}, snapshots.Usage{}, err
	}

	return id, info, usage, nil
}

func (o *snapshotter) getSnapshot(ctx context.Context, key string) (*storage.Snapshot, error) {
	ctx, t, err := o.ms.TransactionContext(ctx, false)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := t.Rollback(); err != nil {
			log.L.WithError(err).Errorf("Rollback traction %s", key)
		}
	}()

	s, err := storage.GetSnapshot(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "get snapshot")
	}

	return &s, nil
}

func (o *snapshotter) mountWithGuestPull(ctx context.Context, s storage.Snapshot) ([]mount.Mount, error) {
	var overlayOptions []string

	log.G(ctx).Infof("s.kind %v", s.Kind)
	if s.Kind == snapshots.KindActive {
		overlayOptions = append(overlayOptions,
			fmt.Sprintf("workdir=%s", o.workPath(s.ID)),
			fmt.Sprintf("upperdir=%s", o.upperPath(s.ID)),
		)
	}
	os.Mkdir(filepath.Join(o.root, "snapshots", s.ID, "low"), 0755)
	overlayOptions = append(overlayOptions, fmt.Sprintf("lowerdir=%s", filepath.Join(o.root, "snapshots", s.ID, "low")))
	log.G(ctx).Infof("remote mount options %v", overlayOptions)
	opt, err := guestpull.PrepareGuestPullMounts(ctx, "", overlayOptions, map[string]string{})
	overlayOptions = append(overlayOptions, opt...)
	if err != nil {
		return nil, err
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
