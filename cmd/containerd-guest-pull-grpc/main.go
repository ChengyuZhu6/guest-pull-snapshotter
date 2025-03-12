package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/v2/contrib/snapshotservice"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/log"
	"github.com/pelletier/go-toml"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"github.com/ChengyuZhu6/guest-pull-snapshotter/config"
	"github.com/ChengyuZhu6/guest-pull-snapshotter/snapshot"
	"github.com/ChengyuZhu6/guest-pull-snapshotter/version"
)

func main() {
	flag.Parse()

	log.SetFormat(log.JSONFormat)
	if err := log.SetLevel(*config.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set log level: %v\n", err)
		os.Exit(1)
	}

	if *config.PrintVersion {
		fmt.Printf("containerd-guest-pull-grpc %s %s\n", version.Version, version.Revision)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = log.WithLogger(ctx, log.L)

	remoteConfig, err := loadConfig(ctx, *config.ConfigPath)
	if err != nil {
		log.G(ctx).WithError(err).Fatal("failed to load configuration")
	}
	log.G(ctx).Infof("remote config loaded: %+v", remoteConfig)

	snapshotter, err := createSnapshotter(ctx, *config.RootDir)
	if err != nil {
		log.G(ctx).WithError(err).Fatal("failed to create snapshotter")
	}
	defer snapshotter.Close()

	rpc := grpc.NewServer()
	if err := startServer(ctx, rpc, *config.Address, snapshotter, cancel); err != nil {
		log.G(ctx).WithError(err).Fatal("server error")
	}

	log.G(ctx).Info("service exited successfully")
}

// loadConfig loads configuration from the specified path
func loadConfig(ctx context.Context, configPath string) (config.RemoteConfig, error) {
	var remoteConfig config.RemoteConfig

	tree, err := toml.LoadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) && configPath == config.DefaultConfigPath {
			log.G(ctx).Warn("config file not found, using defaults")
			return remoteConfig, nil
		}
		return remoteConfig, fmt.Errorf("failed to load config file %q: %w", configPath, err)
	}

	if err := tree.Unmarshal(&remoteConfig); err != nil {
		return remoteConfig, fmt.Errorf("failed to unmarshal config file %q: %w", configPath, err)
	}

	return remoteConfig, nil
}

// createSnapshotter creates and initializes a snapshotter
func createSnapshotter(ctx context.Context, rootDir string) (snapshots.Snapshotter, error) {
	var opts []snapshot.Opt
	snapshotter, err := snapshot.NewSnapshotter(ctx, rootDir, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshotter: %w", err)
	}
	return snapshotter, nil
}

// startServer starts the gRPC server and handles signals
func startServer(ctx context.Context, rpc *grpc.Server, addr string, snapshotter snapshots.Snapshotter, cancel context.CancelFunc) error {
	// Register snapshot service
	snsvc := snapshotservice.FromSnapshotter(snapshotter)
	snapshotsapi.RegisterSnapshotsServer(rpc, snsvc)

	socketDir := filepath.Dir(addr)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", socketDir, err)
	}

	if err := os.RemoveAll(addr); err != nil {
		return fmt.Errorf("failed to remove existing socket %q: %w", addr, err)
	}

	listener, err := net.Listen("unix", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %q: %w", addr, err)
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, unix.SIGINT, unix.SIGTERM, syscall.SIGPIPE)

	// Start server
	errCh := make(chan error, 1)
	go func() {
		log.G(ctx).Infof("starting gRPC server on %q", addr)
		if err := rpc.Serve(listener); err != nil {
			errCh <- err
		}
	}()

	select {
	case sig := <-signalCh:
		log.G(ctx).Infof("received signal: %v", sig)
		rpc.GracefulStop()
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		log.G(ctx).Info("context canceled")
		rpc.Stop()
	}

	return nil
}
