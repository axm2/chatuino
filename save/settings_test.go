package save

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettings_ValidateGraphics(t *testing.T) {
	t.Parallel()

	for _, mode := range EmoteAnimationModes {
		s := BuildDefaultSettings()
		s.Chat.EmoteAnimation = mode
		require.NoError(t, s.validate(), "mode %q", mode)
	}

	s := BuildDefaultSettings()
	s.Chat.EmoteAnimation = "fast"
	require.ErrorContains(t, s.validate(), "chat.emote_animation")

	for _, scale := range []int{0, 1, 4} {
		s := BuildDefaultSettings()
		s.Chat.GraphicScale = scale
		require.NoError(t, s.validate(), "scale %d", scale)
	}

	for _, scale := range []int{-1, 5} {
		s := BuildDefaultSettings()
		s.Chat.GraphicScale = scale
		require.ErrorContains(t, s.validate(), "chat.graphic_scale")
	}
}
