package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/v2/contrib/snapshotservice"
	"github.com/containerd/containerd/v2/core/snapshots"
	"golang.org/x/sys/unix"

	"github.com/containerd/log"
	"github.com/pelletier/go-toml"
	"google.golang.org/grpc"

	"github.com/ChengyuZhu6/guest-pull-snapshotter/config"
	"github.com/ChengyuZhu6/guest-pull-snapshotter/snapshot"
	"github.com/ChengyuZhu6/guest-pull-snapshotter/version"
)

func main() {
	flag.Parse()

	log.SetFormat(log.JSONFormat)
	err := log.SetLevel(*config.LogLevel)
	if err != nil {
		log.L.WithError(err).Fatal("failed to prepare logger")
	}
	if *config.PrintVersion {
		fmt.Println("containerd-guest-pull-grpc", version.Version, version.Revision)
		return
	}

	var (
		ctx          = log.WithLogger(context.Background(), log.L)
		remoteconfig = config.RemoteConfig{}
	)

	tree, err := toml.LoadFile(*config.ConfigPath)
	if err != nil && !(os.IsNotExist(err) && *config.ConfigPath == config.DefaultConfigPath) {
		log.G(ctx).WithError(err).Fatalf("failed to load config file %q", *config.ConfigPath)
	}
	if err := tree.Unmarshal(&remoteconfig); err != nil {
		log.G(ctx).WithError(err).Fatalf("failed to unmarshal config file %q", *config.ConfigPath)
	}
	log.G(ctx).Infof("remoteconfig: %+v", remoteconfig)

	// Create a gRPC server
	rpc := grpc.NewServer()

	var rs snapshots.Snapshotter
	var opts []snapshot.Opt
	rs, err = snapshot.NewSnapshotter(ctx, *config.RootDir, opts...)
	if err != nil {
		return
	}
	cleanup, err := serve(ctx, rpc, *config.Address, rs)
	if err != nil {
		log.G(ctx).WithError(err).Fatalf("failed to serve snapshotter")
	}

	if cleanup {
		log.G(ctx).Debug("Closing the snapshotter")
		rs.Close()
	}
	log.G(ctx).Info("Exiting")
}

func serve(ctx context.Context, rpc *grpc.Server, addr string, rs snapshots.Snapshotter) (bool, error) {
	// Convert the snapshotter to a gRPC service,
	snsvc := snapshotservice.FromSnapshotter(rs)

	// Register the service with the gRPC server
	snapshotsapi.RegisterSnapshotsServer(rpc, snsvc)
	// Prepare the directory for the socket
	if err := os.MkdirAll(filepath.Dir(addr), 0700); err != nil {
		return false, fmt.Errorf("failed to create directory %q: %w", filepath.Dir(addr), err)
	}

	// Try to remove the socket file to avoid EADDRINUSE
	if err := os.RemoveAll(addr); err != nil {
		return false, fmt.Errorf("failed to remove %q: %w", addr, err)
	}

	// Listen and serve
	errCh := make(chan error, 1)
	l, err := net.Listen("unix", addr)
	if err != nil {
		return false, fmt.Errorf("error on listen socket %q: %w", addr, err)
	}
	go func() {
		if err := rpc.Serve(l); err != nil {
			errCh <- fmt.Errorf("error on serving via socket %q: %w", addr, err)
		}
	}()

	var s os.Signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM)
	select {
	case s = <-sigCh:
		log.G(ctx).Infof("Got %v", s)
	case err := <-errCh:
		return false, err
	}
	if s == unix.SIGINT {
		log.G(ctx).Infof("Got SIGINT")
	}
	if s == unix.SIGTERM {
		log.G(ctx).Infof("Got SIGTERM")
	}
	return false, nil
}
