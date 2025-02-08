package config

import (
	"flag"

	"github.com/containerd/log"
)

const (
	DefaultAddress             = "/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock"
	DefaultConfigPath          = "/etc/containerd-guest-pull-grpc/config.toml"
	DefaultLogLevel            = log.InfoLevel
	DefaultRootDir             = "/var/lib/containerd-guest-pull-grpc"
	DefaultImageServiceAddress = "/run/containerd/containerd.sock"
)

var (
	Address      = flag.String("address", DefaultAddress, "address for the snapshotter's GRPC server")
	ConfigPath   = flag.String("config", DefaultConfigPath, "path to the configuration file")
	LogLevel     = flag.String("log-level", DefaultLogLevel.String(), "set the logging level [trace, debug, info, warn, error, fatal, panic]")
	RootDir      = flag.String("root", DefaultRootDir, "path to the root directory for this snapshotter")
	PrintVersion = flag.Bool("version", false, "print the version")
)

// Configure how guest-pull-snapshotter receive auth information
type AuthConfig struct {
	// based on kubeconfig or ServiceAccount
	EnableKubeconfigKeychain bool   `toml:"enable_kubeconfig_keychain"`
	KubeconfigPath           string `toml:"kubeconfig_path"`
	// CRI proxy mode
	EnableCRIKeychain   bool   `toml:"enable_cri_keychain"`
	ImageServiceAddress string `toml:"image_service_address"`
}

type RemoteConfig struct {
	AuthConfig         AuthConfig `toml:"auth"`
	ConvertVpcRegistry bool       `toml:"convert_vpc_registry"`
	SkipSSLVerify      bool       `toml:"skip_ssl_verify"`
}
