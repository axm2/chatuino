package sixelimg

import (
	"bytes"
	"image"
	"io"
	"os"
	"testing"

	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/syncmap"
)

func TestDisplayManager_Convert_FreshDownload(t *testing.T) {
	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, 10, 20)

	emoteData, err := os.ReadFile("../emote/testdata/pepeLaugh.webp")
	require.NoError(t, err)

	unit := kittyimg.DisplayUnit{
		ID:         "fresh-emote",
		Directory:  "emote",
		IsAnimated: false,
		Load: func() (io.ReadCloser, string, error) {
			return io.NopCloser(bytes.NewReader(emoteData)), "image/webp", nil
		},
	}

	result, err := dm.Convert(unit)
	require.NoError(t, err)

	require.NotEmpty(t, result.ReplacementText)
	// Sixel data should start with the sixel escape sequence
	require.Contains(t, result.ReplacementText, "\x1bP")
	// PrepareCommand should be empty for sixel (no pre-transmission needed)
	require.Empty(t, result.PrepareCommand)
}

func TestDisplayManager_Convert_SessionCache(t *testing.T) {
	// Reset global state for this test
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, 10, 20)

	emoteData, err := os.ReadFile("../emote/testdata/pepeLaugh.webp")
	require.NoError(t, err)

	loadCalls := 0
	unit := kittyimg.DisplayUnit{
		ID:         "test-emote",
		Directory:  "emote",
		IsAnimated: false,
		Load: func() (io.ReadCloser, string, error) {
			loadCalls++
			return io.NopCloser(bytes.NewReader(emoteData)), "image/webp", nil
		},
	}

	// First conversion - should load
	result1, err := dm.Convert(unit)
	require.NoError(t, err)
	require.NotEmpty(t, result1.ReplacementText)
	require.Equal(t, 1, loadCalls)

	// Second conversion - should use session cache (no additional load)
	result2, err := dm.Convert(unit)
	require.NoError(t, err)
	require.Equal(t, result1.ReplacementText, result2.ReplacementText)
	require.Equal(t, 1, loadCalls, "should not call Load again from session cache")
}

func TestDisplayManager_CleanupCommands(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	dm := NewDisplayManager(fs, 10, 20)

	// Sixel doesn't need cleanup commands like Kitty does
	require.Empty(t, dm.CleanupAllImagesCommand())
}

func TestImageToSixel(t *testing.T) {
	t.Parallel()

	emoteData, err := os.ReadFile("../emote/testdata/pepeLaugh.webp")
	require.NoError(t, err)

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(emoteData))
	require.NoError(t, err)

	// Convert to sixel
	sixelData, err := imageToSixel(img)
	require.NoError(t, err)
	require.NotEmpty(t, sixelData)

	// Sixel data should start with the DCS escape sequence
	require.True(t, bytes.HasPrefix([]byte(sixelData), []byte("\x1bP")), "sixel data should start with ESC P")
}
