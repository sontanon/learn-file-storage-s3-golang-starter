package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetVideoAspectRatio(t *testing.T) {
	tests := []struct {
		name        string
		videoPath   string
		expected    string
		expectError bool
	}{
		{
			name:        "horizontal video",
			videoPath:   "samples/boots-video-horizontal.mp4",
			expected:    "16:9",
			expectError: false,
		},
		{
			name:        "vertical video",
			videoPath:   "samples/boots-video-vertical.mp4",
			expected:    "9:16",
			expectError: false,
		},
		{
			name:        "non-existent file",
			videoPath:   "samples/does-not-exist.mp4",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getVideoAspectRatio(tt.videoPath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
