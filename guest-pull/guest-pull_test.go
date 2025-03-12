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

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Execute the function
			ctx := context.Background()
			result, err := PrepareGuestPullMounts(ctx, tc.source, tc.options, tc.labels)

			// Verify no error occurred
			require.NoError(t, err)
			require.Len(t, result, 1, "Expected exactly one result option")

			// Extract the option value
			optionParts := splitOption(t, result[0])
			require.Equal(t, KataVirtualVolumeOptionName, optionParts[0], "Option name mismatch")

			// Decode and unmarshal the option value
			decodedVolume := decodeVolumeOption(t, optionParts[1])

			// Verify the volume configuration matches expected
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

func TestPrepareGuestPullMountsWithNilContext(t *testing.T) {
	// Test with nil context
	source := "/path/to/source"
	options := []string{"ro"}
	labels := map[string]string{"label": "value"}

	// Execute with context.TODO() instead of nil
	result, err := PrepareGuestPullMounts(context.TODO(), source, options, labels)

	// Verify no error occurred
	require.NoError(t, err)
	require.Len(t, result, 1, "Expected exactly one result option")

	// Extract the option value
	optionParts := splitOption(t, result[0])
	require.Equal(t, KataVirtualVolumeOptionName, optionParts[0], "Option name mismatch")

	// Decode and unmarshal the option value
	decodedVolume := decodeVolumeOption(t, optionParts[1])

	// Verify the volume configuration
	assert.Equal(t, KataVirtualVolumeImageGuestPullType, decodedVolume.VolumeType)
	assert.Equal(t, source, decodedVolume.Source)
	assert.Equal(t, options, decodedVolume.Options)
	assert.Equal(t, labels, decodedVolume.ImagePull.Metadata)
}

// Helper function to split an option string into key and value
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

// Helper function to decode and unmarshal a volume option
func decodeVolumeOption(t *testing.T, encodedOption string) KataVirtualVolume {
	t.Helper()
	// Decode base64
	jsonData, err := base64.StdEncoding.DecodeString(encodedOption)
	require.NoError(t, err, "Failed to decode base64 option")

	// Unmarshal JSON
	var volume KataVirtualVolume
	err = json.Unmarshal(jsonData, &volume)
	require.NoError(t, err, "Failed to unmarshal volume JSON")

	return volume
}
