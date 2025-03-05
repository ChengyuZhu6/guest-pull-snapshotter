package guestpull

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/containerd/log"
	"github.com/pkg/errors"
)

const (
	KataVirtualVolumeOptionName         = "io.katacontainers.volume"
	KataVirtualVolumeImageGuestPullType = "image_guest_pull"
)

type ImagePullVolume struct {
	Metadata map[string]string `json:"metadata"`
}

type KataVirtualVolume struct {
	VolumeType string           `json:"volume_type"`
	Source     string           `json:"source,omitempty"`
	FSType     string           `json:"fs_type,omitempty"`
	Options    []string         `json:"options,omitempty"`
	ImagePull  *ImagePullVolume `json:"image_pull,omitempty"`
}

func PrepareGuestPullMounts(ctx context.Context, source string, options []string, labels map[string]string) ([]string, error) {
	volume := &KataVirtualVolume{
		VolumeType: KataVirtualVolumeImageGuestPullType,
		Source:     source,
		FSType:     "",
		Options:    options,
	}
	volume.ImagePull = &ImagePullVolume{Metadata: labels}

	validKataVirtualVolumeJSON, err := json.Marshal(volume)
	if err != nil {
		return nil, errors.Wrapf(err, "marshal KataVirtualVolume object")
	}
	log.G(ctx).Infof("encode kata volume %s", validKataVirtualVolumeJSON)
	option := base64.StdEncoding.EncodeToString(validKataVirtualVolumeJSON)
	opt := fmt.Sprintf("%s=%s", KataVirtualVolumeOptionName, option)
	return []string{opt}, nil
}
