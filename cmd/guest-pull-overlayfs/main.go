package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ChengyuZhu6/guest-pull-snapshotter/version"
	"github.com/containerd/log"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// Kata virtual volume information passed from snapshotter to runtime through containerd.
// Please refer to `KataVirtualVolume` in https://github.com/kata-containers/kata-containers/blob/main/src/libs/kata-types/src/mount.rs
const kataVolumeOptionKey = "io.katacontainers.volume="

/*
containerd run fuse.mount format: nydus-overlayfs overlay /tmp/ctd-volume107067851
-o lowerdir=/foo/lower2:/foo/lower1,upperdir=/foo/upper,workdir=/foo/work,extraoption={...},dev,suid]
*/
type mountArgs struct {
	fsType  string
	target  string
	options []string
}

// parseArgs parses command line arguments into mountArgs structure
func parseArgs(args []string) (*mountArgs, error) {

	if len(args) < 2 {
		return nil, errors.New("insufficient arguments for mount")
	}

	margs := &mountArgs{
		fsType: args[0],
		target: args[1],
	}
	if margs.fsType != "overlay" {
		return nil, errors.Errorf("invalid filesystem type %s for overlayfs", margs.fsType)
	}

	if len(margs.target) == 0 {
		return nil, errors.New("empty overlayfs mount target")
	}

	// Check for options
	if len(args) > 2 && args[2] == "-o" && len(args) > 3 && len(args[3]) != 0 {
		for _, opt := range strings.Split(args[3], ",") {
			// filter guestpull specific options
			if strings.HasPrefix(opt, kataVolumeOptionKey) {
				continue
			}
			margs.options = append(margs.options, opt)
		}
	}

	if len(margs.options) == 0 {
		return nil, errors.New("empty overlayfs mount options")
	}

	return margs, nil
}

// parseOptions converts mount options to flags and data string
func parseOptions(options []string) (int, string) {
	flagsTable := map[string]int{
		"async":         unix.MS_SYNCHRONOUS,
		"atime":         unix.MS_NOATIME,
		"bind":          unix.MS_BIND,
		"defaults":      0,
		"dev":           unix.MS_NODEV,
		"diratime":      unix.MS_NODIRATIME,
		"dirsync":       unix.MS_DIRSYNC,
		"exec":          unix.MS_NOEXEC,
		"mand":          unix.MS_MANDLOCK,
		"noatime":       unix.MS_NOATIME,
		"nodev":         unix.MS_NODEV,
		"nodiratime":    unix.MS_NODIRATIME,
		"noexec":        unix.MS_NOEXEC,
		"nomand":        unix.MS_MANDLOCK,
		"norelatime":    unix.MS_RELATIME,
		"nostrictatime": unix.MS_STRICTATIME,
		"nosuid":        unix.MS_NOSUID,
		"rbind":         unix.MS_BIND | unix.MS_REC,
		"relatime":      unix.MS_RELATIME,
		"remount":       unix.MS_REMOUNT,
		"ro":            unix.MS_RDONLY,
		"rw":            unix.MS_RDONLY,
		"strictatime":   unix.MS_STRICTATIME,
		"suid":          unix.MS_NOSUID,
		"sync":          unix.MS_SYNCHRONOUS,
	}

	var (
		flags int
		data  []string
	)

	for _, o := range options {
		if f, exist := flagsTable[o]; exist {
			flags |= f
		} else {
			data = append(data, o)
		}
	}

	return flags, strings.Join(data, ",")
}

// run performs the mount operation with the provided arguments
func run(args []string) error {
	margs, err := parseArgs(args)
	if err != nil {
		return errors.Wrap(err, "parse mount options")
	}

	flags, data := parseOptions(margs.options)
	log.L.Infof("fsType: %v, target: %v, flags: %v, data: %v", margs.fsType, margs.target, uintptr(flags), data)

	err = unix.Mount(margs.fsType, margs.target, margs.fsType, uintptr(flags), data)
	if err != nil {
		return errors.Wrapf(err, "mount overlayfs by syscall")
	}

	return nil
}

func main() {
	if err := log.SetFormat(log.JSONFormat); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set log format: %v\n", err)
		os.Exit(1)
	}

	logLevel := flag.String("log-level", "info", "Set the logging level [trace, debug, info, warn, error, fatal, panic]")
	printVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if err := log.SetLevel(*logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set log level: %v\n", err)
		os.Exit(1)
	}

	if *printVersion {
		fmt.Printf("guest-pull-overlayfs %s %s (built %s)\n",
			version.Version,
			version.Revision,
			version.BuildTimestamp)
		return
	}

	args := flag.Args()
	if len(args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: guest-pull-overlayfs overlay <target> -o <options>\n")
		os.Exit(1)
	}

	err := run(args)
	if err != nil {
		log.L.WithError(err).Fatal("failed to run guest-pull-overlayfs")
	}

	os.Exit(0)
}
