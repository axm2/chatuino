package kittyimg

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// AnimationMode controls how animated images are played.
type AnimationMode int

const (
	// AnimationNative sends all frames to the terminal, which plays them (kitty).
	AnimationNative AnimationMode = iota
	// AnimationClient sends only the first frame, Chatuino swaps frames itself.
	// For terminals without native animation support (Ghostty).
	AnimationClient
	// AnimationOff displays the first frame only.
	AnimationOff
)

const (
	// maxImageFPS caps how often a single image is updated. Frames in between are skipped,
	// so playback speed stays correct.
	maxImageFPS = 15
	// maxFramesPerSecond caps the total frame updates sent to the terminal. Each frame update
	// costs roughly 0.015% terminal CPU (measured in Ghostty), 600/s is about 10%.
	maxFramesPerSecond = 600
	// idleAnimationInterval is the tick interval while no animated image is visible.
	idleAnimationInterval = 250 * time.Millisecond
	// minFrameDelay is the delay below which frame delays are considered broken and
	// replaced with defaultFrameDelay, like browsers do.
	minFrameDelay     = 20 * time.Millisecond
	defaultFrameDelay = 100 * time.Millisecond
)

type animatedImage struct {
	id      int32
	frames  []DecodedImageFrame
	ends    []time.Duration // cumulative end offset of each frame within one loop
	total   time.Duration   // duration of one loop
	start   time.Time
	current int // frame currently transmitted to the terminal
	visible bool
}

// frameAt returns the frame index to display at now.
func (a *animatedImage) frameAt(now time.Time) int {
	offset := now.Sub(a.start) % a.total
	if offset < 0 {
		offset += a.total
	}

	// first frame whose end lies after offset
	return sort.Search(len(a.ends), func(i int) bool { return a.ends[i] > offset })
}

// animator swaps frames of animated images by retransmitting frame data under the same
// image ID. The unicode placeholders stay untouched, so no re-render is necessary.
// Only images visible in the last rendered view are animated.
type animator struct {
	mu      sync.Mutex
	images  map[int32]*animatedImage
	visible map[int32]struct{} // scratch, reused between scans
}

func newAnimator() *animator {
	return &animator{
		images:  map[int32]*animatedImage{},
		visible: map[int32]struct{}{},
	}
}

func (a *animator) register(img DecodedImage, now time.Time) {
	if len(img.Images) < 2 {
		return
	}

	entry := &animatedImage{
		id:     img.ID,
		frames: img.Images,
		ends:   make([]time.Duration, len(img.Images)),
		start:  now,
	}

	for i, frame := range img.Images {
		delay := time.Duration(frame.DelayInMS) * time.Millisecond
		if delay < minFrameDelay {
			delay = defaultFrameDelay
		}
		entry.total += delay
		entry.ends[i] = entry.total
	}

	a.mu.Lock()
	a.images[img.ID] = entry
	a.mu.Unlock()
}

func (a *animator) unregister(id int32) {
	a.mu.Lock()
	delete(a.images, id)
	a.mu.Unlock()
}

// setVisible marks animated images as visible if their placeholders are contained in view.
func (a *animator) setVisible(view string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.images) == 0 {
		return
	}

	clear(a.visible)
	scanPlaceholderImageIDs(view, func(id int32) {
		a.visible[id] = struct{}{}
	})

	for id, img := range a.images {
		_, img.visible = a.visible[id]
	}
}

// frame returns the terminal commands advancing all visible animated images to their
// current frame, and the duration after which frame should be called again.
func (a *animator) frame(now time.Time) (string, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var (
		b       strings.Builder
		visible int
	)

	for _, img := range a.images {
		if !img.visible {
			continue
		}
		visible++

		idx := img.frameAt(now)
		if idx == img.current {
			continue
		}

		img.current = idx
		f := img.frames[idx]
		fmt.Fprintf(&b, "\x1b_Gf=32,i=%d,t=f,q=2,s=%d,v=%d,o=z;%s\x1b\\", img.id, f.Width, f.Height, f.EncodedPath)
	}

	if visible == 0 {
		return b.String(), idleAnimationInterval
	}

	// each visible image is updated at most once per tick, so the tick interval bounds the
	// total frame rate
	interval := max(time.Second/maxImageFPS, time.Duration(visible)*time.Second/maxFramesPerSecond)

	return b.String(), interval
}

// scanPlaceholderImageIDs calls fn for each image ID found in s. Image IDs are encoded as the
// 24-bit foreground color directly preceding a unicode placeholder.
func scanPlaceholderImageIDs(s string, fn func(id int32)) {
	const (
		prefix      = "\x1b[38;2;"
		maxParamLen = len("255;255;255")
	)

	for {
		i := strings.Index(s, prefix)
		if i == -1 {
			return
		}
		s = s[i+len(prefix):]

		end := strings.IndexByte(s, 'm')
		if end == -1 {
			return
		}
		if end > maxParamLen || !strings.HasPrefix(s[end+1:], string(placeholderRune)) {
			continue
		}

		if id, ok := parseRGBID(s[:end]); ok {
			fn(id)
		}
		s = s[end+1:]
	}
}

// parseRGBID parses "R;G;B" into the image ID encoded by intToRGB.
func parseRGBID(params string) (int32, bool) {
	var (
		id        int32
		component int32
		parts     int
		digits    int
	)

	for i := 0; i <= len(params); i++ {
		if i == len(params) || params[i] == ';' {
			if digits == 0 || component > 255 {
				return 0, false
			}
			id = id<<8 | component
			component, digits = 0, 0
			parts++
			continue
		}

		c := params[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		component = component*10 + int32(c-'0')
		digits++
	}

	return id, parts == 3
}
