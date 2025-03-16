package guestpull

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareGuestPullMounts(t *testing.T) {
	// Test cases
	testCases := []struct {
		name     string
		source   string
		options  []string
		labels   map[string]string
		expected KataVirtualVolume
	}{
		{
			name:    "basic configuration",
			source:  "/tmp/source",
			options: []string{"ro", "noexec"},
			labels: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: KataVirtualVolume{
				VolumeType: KataVirtualVolumeImageGuestPullType,
				Source:     "/tmp/source",
				Options:    []string{"ro", "noexec"},
				ImagePull: &ImagePullVolume{
					Metadata: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
		},
		{
			name:    "empty options",
			source:  "/tmp/source",
			options: []string{},
			labels: map[string]string{
				"key1": "value1",
			},
			expected: KataVirtualVolume{
				VolumeType: KataVirtualVolumeImageGuestPullType,
				Source:     "/tmp/source",
				Options:    []string{},
				ImagePull: &ImagePullVolume{
					Metadata: map[string]string{
						"key1": "value1",
					},
				},
			},
		},
		{
			name:    "empty labels",
			source:  "/tmp/source",
			options: []string{"ro"},
			labels:  map[string]string{},
			expected: KataVirtualVolume{
				VolumeType: KataVirtualVolumeImageGuestPullType,
				Source:     "/tmp/source",
				Options:    []string{"ro"},
				ImagePull: &ImagePullVolume{
					Metadata: map[string]string{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := PrepareGuestPullMounts(ctx, tc.source, tc.options, tc.labels)

			require.NoError(t, err)
			require.Len(t, result, 1, "Expected exactly one result option")

			optionParts := splitOption(t, result[0])
			require.Equal(t, KataVirtualVolumeOptionName, optionParts[0], "Option name mismatch")

			decodedVolume := decodeVolumeOption(t, optionParts[1])

			assert.Equal(t, tc.expected.VolumeType, decodedVolume.VolumeType)
			assert.Equal(t, tc.expected.Source, decodedVolume.Source)
			if len(tc.expected.Options) == 0 {
				assert.Empty(t, decodedVolume.Options, "Options should be empty")
			} else {
				assert.Equal(t, tc.expected.Options, decodedVolume.Options)
			}
			assert.True(t, reflect.DeepEqual(tc.expected.ImagePull.Metadata, decodedVolume.ImagePull.Metadata))
		})
	}
}

func splitOption(t *testing.T, option string) []string {
	t.Helper()
	parts := make([]string, 2)
	for i, r := range option {
		if r == '=' {
			parts[0] = option[:i]
			parts[1] = option[i+1:]
			return parts
		}
	}
	t.Fatalf("Option string does not contain '=': %s", option)
	return nil
}

func decodeVolumeOption(t *testing.T, encodedOption string) KataVirtualVolume {
	t.Helper()
	jsonData, err := base64.StdEncoding.DecodeString(encodedOption)
	require.NoError(t, err, "Failed to decode base64 option")

	var volume KataVirtualVolume
	err = json.Unmarshal(jsonData, &volume)
	require.NoError(t, err, "Failed to unmarshal volume JSON")

	return volume
}
