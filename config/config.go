// Package config provides configuration options for the guest-pull-snapshotter
package config

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/containerd/log"
	"github.com/pkg/errors"
)

// Default configuration values
const (
	// DefaultAddress is the default socket path for the snapshotter's GRPC server
	DefaultAddress = "/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock"

	// DefaultConfigPath is the default path to the configuration file
	DefaultConfigPath = "/etc/containerd-guest-pull-grpc/config.toml"

	// DefaultLogLevel is the default logging level
	DefaultLogLevel = log.InfoLevel

	// DefaultRootDir is the default root directory for the snapshotter
	DefaultRootDir = "/var/lib/containerd-guest-pull-grpc"

	// DefaultImageServiceAddress is the default address for the containerd image service
	DefaultImageServiceAddress = "/run/containerd/containerd.sock"
)

// Command line flags
var (
	// Address specifies the socket path for the snapshotter's GRPC server
	Address = flag.String("address", getEnvOrDefault("GUEST_PULL_ADDRESS", DefaultAddress),
		"address for the snapshotter's GRPC server")

	// ConfigPath specifies the path to the configuration file
	ConfigPath = flag.String("config", getEnvOrDefault("GUEST_PULL_CONFIG", DefaultConfigPath),
		"path to the configuration file")

	// LogLevel specifies the logging level
	LogLevel = flag.String("log-level", getEnvOrDefault("GUEST_PULL_LOG_LEVEL", DefaultLogLevel.String()),
		"set the logging level [trace, debug, info, warn, error, fatal, panic]")

	// RootDir specifies the root directory for the snapshotter
	RootDir = flag.String("root", getEnvOrDefault("GUEST_PULL_ROOT", DefaultRootDir),
		"path to the root directory for this snapshotter")

	// PrintVersion indicates whether to print the version and exit
	PrintVersion = flag.Bool("version", false, "print the version")
)

// getEnvOrDefault returns the value of the environment variable if set,
// otherwise returns the default value
func getEnvOrDefault(envVar, defaultValue string) string {
	if val, ok := os.LookupEnv(envVar); ok {
		return val
	}
	return defaultValue
}

// Add validation function for configuration
func ValidateConfig() error {
	if *RootDir == "" {
		return errors.New("root directory must be specified")
	}
	
	if err := os.MkdirAll(*RootDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create root directory %s", *RootDir)
	}
	
	// Test write permissions with a single operation
	testFile := filepath.Join(*RootDir, ".write_test")
	if err := os.WriteFile(testFile, []byte{}, 0600); err != nil {
		return errors.Wrapf(err, "root directory %s is not writable", *RootDir)
	}
	
	return os.Remove(testFile)
}
