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

// Constants for Kata virtual volume configuration:
//
//	https://github.com/kata-containers/kata-containers/blob/main/src/runtime/virtcontainers/pkg/config/config.go#L100
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

// Add validation for the volume configuration
func ValidateVolumeConfig(volume *KataVirtualVolume) error {
	if volume == nil {
		return errors.New("volume configuration cannot be nil")
	}
	
	if volume.VolumeType == "" {
		return errors.New("volume type cannot be empty")
	}
	
	if volume.VolumeType == KataVirtualVolumeImageGuestPullType && volume.ImagePull == nil {
		return errors.New("image pull configuration required for guest pull volume type")
	}
	
	return nil
}

// PrepareGuestPullMounts creates mount options for guest pull operations
// It takes a source path, mount options, and labels, and returns
// a slice of options with the encoded Kata virtual volume configuration.
func PrepareGuestPullMounts(ctx context.Context, source string, options []string, labels map[string]string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	volume := &KataVirtualVolume{
		VolumeType: KataVirtualVolumeImageGuestPullType,
		Source:     source,
		Options:    options,
		ImagePull: &ImagePullVolume{
			Metadata: labels,
		},
	}

	if err := ValidateVolumeConfig(volume); err != nil {
		return nil, errors.Wrap(err, "invalid volume configuration")
	}

	volumeJSON, err := json.Marshal(volume)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal volume configuration")
	}

	encodedVolume := base64.StdEncoding.EncodeToString(volumeJSON)
	optionString := fmt.Sprintf("%s=%s", KataVirtualVolumeOptionName, encodedVolume)
	log.G(ctx).WithField("option", optionString).Debug("prepared guest pull mount option")

	return []string{optionString}, nil
}
