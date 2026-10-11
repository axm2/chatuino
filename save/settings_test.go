package save

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettings_ValidateEmoteAnimation(t *testing.T) {
	t.Parallel()

	for _, mode := range EmoteAnimationModes {
		s := BuildDefaultSettings()
		s.Chat.EmoteAnimation = mode
		require.NoError(t, s.validate(), "mode %q", mode)
	}

	s := BuildDefaultSettings()
	s.Chat.EmoteAnimation = "fast"
	require.ErrorContains(t, s.validate(), "chat.emote_animation")
}
