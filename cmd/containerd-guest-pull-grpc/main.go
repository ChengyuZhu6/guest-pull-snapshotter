package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/v2/contrib/snapshotservice"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/ChengyuZhu6/guest-pull-snapshotter/pkg/snapshots"
)

var (
	rootDir    string
	address    string
	configPath string
)

func init() {
	flag.StringVar(&rootDir, "root", "/var/lib/containerd-guest-pull-grpc", "Root directory for snapshotter")
	flag.StringVar(&address, "address", "/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock", "Address for GRPC server")
	flag.StringVar(&configPath, "config", "", "Path to config file")
}

func main() {
	flag.Parse()

	log := logrus.New()

	// Create root directory if it doesn't exist
	if err := os.MkdirAll(rootDir, 0700); err != nil {
		log.WithError(err).Fatal("failed to create root directory")
	}

	// Create snapshotter
	sn, err := snapshots.New(rootDir)
	if err != nil {
		log.WithError(err).Fatal("failed to create snapshotter")
	}

	// Create GRPC server
	rpc := grpc.NewServer()

	// Register service
	service := snapshotservice.FromSnapshotter(sn)
	snapshotsapi.RegisterSnapshotsServer(rpc, service)

	// Create UNIX socket
	if err := os.MkdirAll(filepath.Dir(address), 0700); err != nil {
		log.WithError(err).Fatal("failed to create socket directory")
	}

	// Remove socket file if it already exists
	if err := os.RemoveAll(address); err != nil {
		log.WithError(err).Fatal("failed to remove existing socket")
	}

	// Listen on socket
	l, err := net.Listen("unix", address)
	if err != nil {
		log.WithError(err).Fatal("failed to listen on socket")
	}

	// Handle shutdown gracefully
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		rpc.GracefulStop()
	}()

	// Start server
	log.Infof("starting GRPC server on %s", address)
	if err := rpc.Serve(l); err != nil {
		log.WithError(err).Fatal("GRPC server failed")
	}
}
