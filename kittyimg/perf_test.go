package kittyimg

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// perfSetup skips perf tests unless CHATUINO_PERF is set, they are slow (especially with -race).
func perfSetup(t *testing.T) {
	if os.Getenv("CHATUINO_PERF") == "" {
		t.Skip("perf test, set CHATUINO_PERF=1 to run")
	}
	zerolog.SetGlobalLevel(zerolog.Disabled) // Convert logs every download
}

// Performance tests for the image pipeline. They answer: how long does Convert block its
// caller, and what does that do to client side animations driven by the same event loop?
//
//	CHATUINO_PERF=1 go test ./kittyimg -run TestPerf -v
//	go test ./kittyimg -run '^$' -bench . -benchmem

type perfFixture struct {
	name        string
	path        string
	contentType string
	animated    bool
}

var perfFixtures = []perfFixture{
	{"static_webp", "../emote/testdata/pepeLaugh.webp", "image/webp", false},
	{"animated_webp", "../emote/testdata/animated.webp", "image/webp", true},
	{"animated_avif", "../emote/testdata/test.avif", "image/avif", true},
	{"animated_gif", "../emote/testdata/test.gif", "image/gif", true},
}

// perfFS returns a real on-disk filesystem rooted in a temp dir, so zlib + file writes are
// measured realistically without touching the user's data directory.
func perfFS(tb testing.TB) afero.Fs {
	return afero.NewBasePathFs(afero.NewOsFs(), tb.TempDir())
}

var perfIDCounter atomic.Int64

func perfUnit(tb testing.TB, f perfFixture, latency time.Duration) DisplayUnit {
	data, err := os.ReadFile(f.path)
	require.NoError(tb, err)

	return DisplayUnit{
		ID:         fmt.Sprintf("perf-%s-%d", f.name, perfIDCounter.Add(1)),
		Directory:  "emote",
		IsAnimated: f.animated,
		Load: func() (io.ReadCloser, string, error) {
			time.Sleep(latency) // simulated CDN round trip
			return io.NopCloser(bytes.NewReader(data)), f.contentType, nil
		},
	}
}

// TestPerf_ConvertLatency reports how long a single Convert blocks for each cache tier.
// Every tier runs on the bubbletea event loop today (Root.Update -> buildChatEventMessage).
func TestPerf_ConvertLatency(t *testing.T) {
	perfSetup(t)

	dm := NewDisplayManager(perfFS(t), 9, 18, DisplayOptions{Animation: AnimationClient})

	for _, f := range perfFixtures {
		unit := perfUnit(t, f, 0)

		start := time.Now()
		_, err := dm.Convert(unit)
		coldDur := time.Since(start)
		require.NoError(t, err)

		placed, ok := globalPlacedImages.Load(unit.ID)
		require.True(t, ok)
		frames := len(placed.(DecodedImage).Images)

		start = time.Now()
		_, err = dm.Convert(unit)
		sessionDur := time.Since(start)
		require.NoError(t, err)

		// simulate a new session: drop placement, keep disk cache
		globalPlacedImages.Delete(unit.ID)
		start = time.Now()
		_, err = dm.Convert(unit)
		diskDur := time.Since(start)
		require.NoError(t, err)

		t.Logf("%-14s frames=%-3d cold(decode+encode+write)=%-10v disk-cache=%-10v session-cache=%v",
			f.name, frames, coldDur.Round(time.Microsecond), diskDur.Round(time.Microsecond), sessionDur.Round(time.Microsecond))
	}
}

// TestPerf_EventLoopStall simulates the bubbletea event loop: a single goroutine handles both
// animation ticks and incoming chat messages. Each chat message contains a not yet seen emote,
// so Convert has to download and decode it. The largest gap between animation ticks is what the
// user perceives as a freeze (no typing, no scrolling, no animation).
func TestPerf_EventLoopStall(t *testing.T) {
	perfSetup(t)

	const (
		messages        = 20
		messageInterval = 50 * time.Millisecond
	)

	for _, latency := range []time.Duration{0, 100 * time.Millisecond} {
		for _, f := range perfFixtures {
			for _, async := range []bool{false, true} {
				name := fmt.Sprintf("%s/latency=%v/async=%v", f.name, latency, async)
				dm := NewDisplayManager(perfFS(t), 9, 18, DisplayOptions{Animation: AnimationClient})

				units := make([]DisplayUnit, messages)
				for i := range units {
					units[i] = perfUnit(t, f, latency)
				}

				gaps := runEventLoop(dm, units, messageInterval, async)
				slices.Sort(gaps)

				expected := time.Second / maxImageFPS
				t.Logf("%-52s ticks=%-3d p50=%-8v p99=%-8v max=%-8v (expected %v)",
					name, len(gaps),
					gaps[len(gaps)/2].Round(time.Millisecond),
					gaps[len(gaps)*99/100].Round(time.Millisecond),
					gaps[len(gaps)-1].Round(time.Millisecond),
					expected.Round(time.Millisecond))
			}
		}
	}
}

// runEventLoop returns the gaps between consecutive animation ticks.
func runEventLoop(dm *DisplayManager, units []DisplayUnit, interval time.Duration, async bool) []time.Duration {
	incoming := make(chan DisplayUnit)
	converted := make(chan KittyDisplayUnit, len(units))

	go func() {
		for _, u := range units {
			incoming <- u
			time.Sleep(interval)
		}
		close(incoming)
	}()

	// one visible animated image keeps the loop ticking at maxImageFPS, like an emote on screen
	visible := testAnimatedImage(1<<20, 50, 50, 50, 50)
	dm.animator.register(visible, time.Now())
	dm.TrackVisibleImages(visible.DisplayUnicodePlaceholder())

	var (
		gaps     []time.Duration
		lastTick = time.Now()
		pending  = 0
		tick     = time.NewTimer(time.Second / maxImageFPS)
	)
	defer tick.Stop()

	for incoming != nil || pending > 0 {
		select {
		case u, ok := <-incoming:
			if !ok {
				incoming = nil
				continue
			}
			if async {
				pending++
				go func() {
					res, _ := dm.Convert(u)
					converted <- res
				}()
				continue
			}
			_, _ = dm.Convert(u) // what Root.Update does today
		case <-converted:
			pending--
		case now := <-tick.C:
			gaps = append(gaps, now.Sub(lastTick))
			lastTick = now
			_, next := dm.AnimationFrame(now)
			tick.Reset(next)
		}
	}

	return gaps
}

func BenchmarkConvert_Cold(b *testing.B) {
	for _, f := range perfFixtures {
		b.Run(f.name, func(b *testing.B) {
			dm := NewDisplayManager(perfFS(b), 9, 18, DisplayOptions{Animation: AnimationClient})
			for b.Loop() {
				_, err := dm.Convert(perfUnit(b, f, 0))
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkConvert_DiskCache(b *testing.B) {
	for _, f := range perfFixtures {
		b.Run(f.name, func(b *testing.B) {
			dm := NewDisplayManager(perfFS(b), 9, 18, DisplayOptions{Animation: AnimationClient})
			unit := perfUnit(b, f, 0)
			_, err := dm.Convert(unit)
			require.NoError(b, err)

			for b.Loop() {
				globalPlacedImages.Delete(unit.ID)
				_, err := dm.Convert(unit)
				require.NoError(b, err)
			}
		})
	}
}

// BenchmarkAnimator_Tick measures per tick work on the event loop: scanning the rendered view
// for visible images and building frame commands.
func BenchmarkAnimator_Tick(b *testing.B) {
	for _, n := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("images=%d", n), func(b *testing.B) {
			a := newAnimator()
			var view strings.Builder
			for i := range n {
				img := testAnimatedImage(int32(i+1), 30, 40, 50, 60)
				a.register(img, time.Now())
				view.WriteString("some chat text ")
				view.WriteString(img.DisplayUnicodePlaceholder())
				view.WriteString("\n")
			}
			s := view.String()

			now := time.Now()
			for b.Loop() {
				a.setVisible(s)
				now = now.Add(70 * time.Millisecond)
				_, _ = a.frame(now)
			}
		})
	}
}
