package main

import (
	"os"
	"testing"

	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

// not parallel: tests modify the environment

func TestEmoteAnimationMode_Explicit(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "1")

	require.Equal(t, kittyimg.AnimationNative, emoteAnimationMode(save.EmoteAnimationNative))
	require.Equal(t, kittyimg.AnimationClient, emoteAnimationMode(save.EmoteAnimationClient))
	require.Equal(t, kittyimg.AnimationOff, emoteAnimationMode(save.EmoteAnimationOff))
}

func TestEmoteAnimationMode_Auto(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "1")
	require.Equal(t, kittyimg.AnimationNative, emoteAnimationMode(""))
	require.Equal(t, kittyimg.AnimationNative, emoteAnimationMode(save.EmoteAnimationAuto))

	require.NoError(t, os.Unsetenv("KITTY_WINDOW_ID")) // restored by t.Setenv cleanup
	require.Equal(t, kittyimg.AnimationClient, emoteAnimationMode(""))
	require.Equal(t, kittyimg.AnimationClient, emoteAnimationMode(save.EmoteAnimationAuto))
}
