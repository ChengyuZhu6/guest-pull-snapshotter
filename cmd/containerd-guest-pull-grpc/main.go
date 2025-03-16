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
	"google.golang.org/grpc"

	"github.com/ChengyuZhu6/guest-pull-snapshotter/config"
	"github.com/ChengyuZhu6/guest-pull-snapshotter/snapshot"
	"github.com/ChengyuZhu6/guest-pull-snapshotter/version"
)

func main() {
	flag.Parse()

	if err := log.SetFormat(log.JSONFormat); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set log format: %v\n", err)
		os.Exit(1)
	}

	if err := log.SetLevel(*config.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set log level: %v\n", err)
		os.Exit(1)
	}

	if *config.PrintVersion {
		fmt.Printf("containerd-guest-pull-grpc %s %s (built %s)\n",
			version.Version,
			version.Revision,
			version.BuildTimestamp)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = log.WithLogger(ctx, log.L)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalCh
		log.G(ctx).Infof("Received signal %s, shutting down...", sig)
		cancel()
	}()

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

// createSnapshotter creates and initializes a snapshotter
func createSnapshotter(ctx context.Context, rootDir string) (snapshots.Snapshotter, error) {
	opts := []snapshot.Opt{
		snapshot.WithRootDirectory(rootDir),
	}

	snapshotter, err := snapshot.NewSnapshotter(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshotter: %w", err)
	}
	return snapshotter, nil
}

// startServer starts the gRPC server and handles signals
func startServer(ctx context.Context, rpc *grpc.Server, addr string, snapshotter snapshots.Snapshotter, cancel context.CancelFunc) error {
	snsvc := snapshotservice.FromSnapshotter(snapshotter)
	snapshotsapi.RegisterSnapshotsServer(rpc, snsvc)

	// Prepare socket directory
	socketDir := filepath.Dir(addr)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", socketDir, err)
	}

	// Clean up existing socket
	os.RemoveAll(addr) // Ignore error as it might not exist

	listener, err := net.Listen("unix", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %q: %w", addr, err)
	}

	// Start server
	log.G(ctx).Infof("starting gRPC server on %q", addr)
	go func() {
		if err := rpc.Serve(listener); err != nil && ctx.Err() == nil {
			log.G(ctx).WithError(err).Error("server error")
			cancel()
		}
	}()

	<-ctx.Done()
	log.G(ctx).Info("context canceled")
	rpc.Stop()

	return nil
}
