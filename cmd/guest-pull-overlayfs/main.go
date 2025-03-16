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

type mountArgs struct {
	fsType  string
	target  string
	options []string
}

// parseArgs parses command line arguments into mountArgs structure
func parseArgs(args []string) (*mountArgs, error) {
	if len(args) < 2 {
		return nil, errors.New("insufficient arguments for mount, expected: overlay <target> -o <options>")
	}

	margs := &mountArgs{
		fsType: args[0],
		target: args[1],
	}

	if margs.fsType != "overlay" {
		return nil, errors.Errorf("invalid filesystem type %q, expected 'overlay'", margs.fsType)
	}

	if margs.target == "" {
		return nil, errors.New("empty overlayfs mount target")
	}

	if len(args) > 3 && args[2] == "-o" && args[3] != "" {
		for _, opt := range strings.Split(args[3], ",") {
			if opt == "" || strings.HasPrefix(opt, kataVolumeOptionKey) {
				continue
			}
			margs.options = append(margs.options, opt)
		}
	}

	if len(margs.options) == 0 {
		return nil, errors.New("no valid overlayfs mount options provided")
	}

	return margs, nil
}

func parseOptions(options []string) (int, string) {

	type flagOperation struct {
		flag  int
		clear bool
	}

	optionMap := map[string]flagOperation{
		"async":         {unix.MS_SYNCHRONOUS, true},
		"atime":         {unix.MS_NOATIME, true},
		"dev":           {unix.MS_NODEV, true},
		"diratime":      {unix.MS_NODIRATIME, true},
		"exec":          {unix.MS_NOEXEC, true},
		"suid":          {unix.MS_NOSUID, true},
		"rw":            {unix.MS_RDONLY, true},
		"nomand":        {unix.MS_MANDLOCK, true},
		"norelatime":    {unix.MS_RELATIME, true},
		"nostrictatime": {unix.MS_STRICTATIME, true},
		"bind":        {unix.MS_BIND, false},
		"dirsync":     {unix.MS_DIRSYNC, false},
		"mand":        {unix.MS_MANDLOCK, false},
		"noatime":     {unix.MS_NOATIME, false},
		"nodev":       {unix.MS_NODEV, false},
		"nodiratime":  {unix.MS_NODIRATIME, false},
		"noexec":      {unix.MS_NOEXEC, false},
		"nosuid":      {unix.MS_NOSUID, false},
		"rbind":       {unix.MS_BIND | unix.MS_REC, false},
		"relatime":    {unix.MS_RELATIME, false},
		"remount":     {unix.MS_REMOUNT, false},
		"ro":          {unix.MS_RDONLY, false},
		"strictatime": {unix.MS_STRICTATIME, false},
		"sync":        {unix.MS_SYNCHRONOUS, false},
		"defaults": {0, false},
	}

	var flags int
	var dataOptions []string

	for _, opt := range options {
		if operation, exists := optionMap[opt]; exists {
			if operation.clear {
				flags &= ^operation.flag
			} else if operation.flag != 0 {
				flags |= operation.flag
			}
		} else {
			dataOptions = append(dataOptions, opt)
		}
	}

	data := strings.Join(dataOptions, ",")
	
	return flags, data
}

func run(args []string) error {
	margs, err := parseArgs(args)
	if err != nil {
		return errors.Wrap(err, "failed to parse mount arguments")
	}

	flags, data := parseOptions(margs.options)

	if err := unix.Mount(margs.fsType, margs.target, margs.fsType, uintptr(flags), data); err != nil {
		return errors.Wrapf(err, "failed to mount overlayfs at %q", margs.target)
	}

	log.L.WithField("target", margs.target).Info("successfully mounted overlayfs")
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
