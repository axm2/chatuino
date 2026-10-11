package kittyimg

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/syncmap"
)

func testAnimatedImage(id int32, delays ...int) DecodedImage {
	img := DecodedImage{ID: id, Cols: 1}
	for i, d := range delays {
		img.Images = append(img.Images, DecodedImageFrame{
			Width:       10,
			Height:      10,
			EncodedPath: "frame" + string(rune('0'+i)),
			DelayInMS:   d,
		})
	}
	return img
}

func TestAnimatedImage_FrameAt(t *testing.T) {
	t.Parallel()

	start := time.Unix(1000, 0)
	a := newAnimator()
	a.register(testAnimatedImage(1, 100, 50, 0), start) // 0ms delay is normalized to 100ms
	img := a.images[1]

	require.Equal(t, 250*time.Millisecond, img.total)

	tests := []struct {
		offset time.Duration
		frame  int
	}{
		{0, 0},
		{99 * time.Millisecond, 0},
		{100 * time.Millisecond, 1},
		{149 * time.Millisecond, 1},
		{150 * time.Millisecond, 2},
		{249 * time.Millisecond, 2},
		{250 * time.Millisecond, 0}, // loops
		{360 * time.Millisecond, 1},
	}

	for _, tt := range tests {
		require.Equal(t, tt.frame, img.frameAt(start.Add(tt.offset)), "offset %s", tt.offset)
	}
}

func TestAnimator_StaticImagesNotRegistered(t *testing.T) {
	t.Parallel()

	a := newAnimator()
	a.register(testAnimatedImage(1, 100), time.Now())
	require.Empty(t, a.images)
}

func TestAnimator_OnlyVisibleImagesAnimate(t *testing.T) {
	t.Parallel()

	start := time.Unix(1000, 0)
	a := newAnimator()
	visible := testAnimatedImage(1, 100, 100)
	hidden := testAnimatedImage(2, 100, 100)
	a.register(visible, start)
	a.register(hidden, start)

	// nothing visible yet: idle
	cmd, next := a.frame(start.Add(150 * time.Millisecond))
	require.Empty(t, cmd)
	require.Equal(t, idleAnimationInterval, next)

	a.setVisible("chat line " + visible.DisplayUnicodePlaceholder() + " more text")

	cmd, next = a.frame(start.Add(150 * time.Millisecond))
	require.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10,o=z;frame1\x1b\\", cmd)
	require.Equal(t, time.Second/maxImageFPS, next)

	// same frame, nothing to send
	cmd, _ = a.frame(start.Add(180 * time.Millisecond))
	require.Empty(t, cmd)

	// loops back to first frame
	cmd, _ = a.frame(start.Add(210 * time.Millisecond))
	require.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10,o=z;frame0\x1b\\", cmd)

	// scrolled out of view
	a.setVisible("no images")
	cmd, next = a.frame(start.Add(350 * time.Millisecond))
	require.Empty(t, cmd)
	require.Equal(t, idleAnimationInterval, next)

	a.setVisible(hidden.DisplayUnicodePlaceholder())
	a.unregister(2)
	cmd, next = a.frame(start.Add(350 * time.Millisecond))
	require.Empty(t, cmd)
	require.Equal(t, idleAnimationInterval, next)
}

func TestAnimator_FrameBudget(t *testing.T) {
	t.Parallel()

	start := time.Unix(1000, 0)
	a := newAnimator()

	var view strings.Builder
	for id := int32(1); id <= 120; id++ {
		img := testAnimatedImage(id, 100, 100)
		a.register(img, start)
		view.WriteString(img.DisplayUnicodePlaceholder())
	}
	a.setVisible(view.String())

	cmd, next := a.frame(start.Add(150 * time.Millisecond))
	require.Equal(t, 120, strings.Count(cmd, "\x1b_G"))
	// 120 images, at most maxFramesPerSecond updates per second
	require.Equal(t, 120*time.Second/maxFramesPerSecond, next)
}

func TestScanPlaceholderImageIDs(t *testing.T) {
	t.Parallel()

	multiRow := DecodedImage{ID: 0x7DA943, Cols: 2, Rows: 2}
	single := DecodedImage{ID: 256, Cols: 1}

	view := strings.Join([]string{
		"\x1b[38;2;1;2;3muser\x1b[39m: hi " + single.DisplayUnicodePlaceholder(), // user color is not an image
		strings.Join(PlaceholderRowsAbove(multiRow.DisplayUnicodePlaceholder(), 2), "\n"),
		multiRow.DisplayUnicodePlaceholder(),
		"\x1b[38;2;300;0;0m\U0010EEEE", // invalid component
		"\x1b[38;2;1;2m\U0010EEEE",     // missing component
		"\x1b[38;2;0;0;7m",             // truncated at end
	}, "\n")

	var ids []int32
	scanPlaceholderImageIDs(view, func(id int32) { ids = append(ids, id) })
	require.Equal(t, []int32{256, 0x7DA943, 0x7DA943}, ids)
}

func TestDisplayManager_AnimationModes(t *testing.T) {
	// modifies global session cache
	animated := encodeTestGIF(t)

	tests := []struct {
		mode          AnimationMode
		wantFrames    bool // all frames sent to terminal
		wantAnimating bool // registered for client side animation
	}{
		{AnimationNative, true, false},
		{AnimationClient, false, true},
		{AnimationOff, false, false},
	}

	for _, tt := range tests {
		resetGlobalImageState()

		dm := NewDisplayManager(afero.NewMemMapFs(), 10, 10, DisplayOptions{Rows: 1, Animation: tt.mode})
		require.Equal(t, tt.mode == AnimationClient, dm.AnimatesClientSide())

		unit, err := dm.Convert(DisplayUnit{
			ID:         "animated",
			Directory:  "emote",
			IsAnimated: true,
			Load: func() (io.ReadCloser, string, error) {
				return io.NopCloser(bytes.NewReader(animated)), "image/gif", nil
			},
		})
		require.NoError(t, err)

		require.Equal(t, tt.wantFrames, strings.Contains(unit.PrepareCommand, "a=f"), "mode %d", tt.mode)
		require.Contains(t, unit.PrepareCommand, "a=p", "mode %d", tt.mode)
		require.Len(t, dm.animator.images, map[bool]int{true: 1, false: 0}[tt.wantAnimating], "mode %d", tt.mode)

		// cleanup unregisters
		dm.CleanupOldImagesCommand(-time.Second)
		require.Empty(t, dm.animator.images)
	}
}

func encodeTestGIF(t *testing.T) []byte {
	t.Helper()

	palette := color.Palette{color.Black, color.White}
	anim := &gif.GIF{}
	for i := range 3 {
		frame := image.NewPaletted(image.Rect(0, 0, 4, 4), palette)
		frame.SetColorIndex(i, i, 1)
		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, 10)
	}

	var buf bytes.Buffer
	require.NoError(t, gif.EncodeAll(&buf, anim))
	return buf.Bytes()
}

func resetGlobalImageState() {
	globalImagePlacementIDCounter.Store(0)
	globalPlacedImages = &syncmap.Map{}
}
