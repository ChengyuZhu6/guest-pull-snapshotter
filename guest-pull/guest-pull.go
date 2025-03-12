// Package guestpull provides functionality for handling guest pull operations
// in Kata Containers virtual environments.
package guestpull

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/containerd/log"
	"github.com/pkg/errors"
)

// Constants for Kata virtual volume configuration
const (
	// KataVirtualVolumeOptionName is the option key for Kata virtual volumes
	KataVirtualVolumeOptionName = "io.katacontainers.volume"

	// KataVirtualVolumeImageGuestPullType defines the volume type for guest pull operations
	KataVirtualVolumeImageGuestPullType = "image_guest_pull"
)

// ImagePullVolume represents the metadata for an image pull volume
type ImagePullVolume struct {
	Metadata map[string]string `json:"metadata"`
}

// KataVirtualVolume represents the configuration for a Kata virtual volume
type KataVirtualVolume struct {
	VolumeType string           `json:"volume_type"`
	Source     string           `json:"source,omitempty"`
	FSType     string           `json:"fs_type,omitempty"`
	Options    []string         `json:"options,omitempty"`
	ImagePull  *ImagePullVolume `json:"image_pull,omitempty"`
}

// PrepareGuestPullMounts creates mount options for guest pull operations
// It takes a source path, mount options, and labels, and returns
// a slice of options with the encoded Kata virtual volume configuration.
func PrepareGuestPullMounts(ctx context.Context, source string, options []string, labels map[string]string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Create and populate the virtual volume configuration
	volume := &KataVirtualVolume{
		VolumeType: KataVirtualVolumeImageGuestPullType,
		Source:     source,
		Options:    options,
		ImagePull:  &ImagePullVolume{Metadata: labels},
	}

	// Marshal the volume configuration to JSON
	volumeJSON, err := json.Marshal(volume)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal KataVirtualVolume object")
	}

	log.G(ctx).Debugf("Kata virtual volume configuration: %s", volumeJSON)
	encodedOption := base64.StdEncoding.EncodeToString(volumeJSON)
	kataOption := fmt.Sprintf("%s=%s", KataVirtualVolumeOptionName, encodedOption)

	return []string{kataOption}, nil
}
