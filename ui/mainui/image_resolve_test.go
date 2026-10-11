package mainui

import (
	"strings"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func testDeps() *DependencyContainer {
	return &DependencyContainer{
		UserConfig: UserConfiguration{
			Settings: save.BuildDefaultSettings(),
			Theme:    save.BuildDefaultTheme(),
		},
		Keymap: save.BuildDefaultKeyMap(),
	}
}

func testPrivateMessage(id, text string) *twitchirc.PrivateMessage {
	return &twitchirc.PrivateMessage{
		ID:              id,
		LoginName:       "user",
		DisplayName:     "User",
		ChannelUserName: "channel",
		RoomID:          "1",
		Message:         text,
		TMISentTS:       time.Unix(0, 0),
	}
}

type fakeEmoteReplacer struct {
	cached, resolved map[string]string
	pending          bool
}

func (f fakeEmoteReplacer) Replace(string, string, []twitchirc.Emote) (string, map[string]string, error) {
	return "prepare-emote", f.resolved, nil
}

func (f fakeEmoteReplacer) ReplaceCached(string, string, []twitchirc.Emote) (string, map[string]string, bool, error) {
	return "", f.cached, f.pending, nil
}

type fakeBadgeReplacer struct{}

func (fakeBadgeReplacer) Replace(string, []twitchirc.Badge) (string, map[string]string, error) {
	return "", map[string]string{}, nil
}

func (fakeBadgeReplacer) ReplaceCached(string, []twitchirc.Badge) (string, map[string]string, bool, error) {
	return "", map[string]string{}, false, nil
}

func (fakeBadgeReplacer) InjectContributorBadge(string, map[string]string) (string, error) {
	return "", nil
}

func TestChatWindow_ImagesResolved(t *testing.T) {
	t.Parallel()

	c := newChatWindow(40, 5, testDeps())

	long := strings.Repeat("word ", 12) + "pepeLaugh"
	c.handleMessage(chatEventMessage{message: testPrivateMessage("1", "first")})
	c.handleMessage(chatEventMessage{
		message:  testPrivateMessage("2", long),
		imageKey: 7,
		displayModifier: messageContentModifier{
			wordReplacements: wordReplacement{"pepeLaugh": "pepeLaugh"},
		},
	})
	c.handleMessage(chatEventMessage{message: testPrivateMessage("3", "third")})

	before := len(c.lines)

	// unrelated key is ignored
	c, _ = c.Update(imagesResolvedMessage{imageKey: 8, wordReplacements: wordReplacement{"pepeLaugh": "X"}})
	require.Equal(t, before, len(c.lines))
	require.False(t, strings.Contains(strings.Join(c.lines, "\n"), "X"))

	// replacement much longer than the text, so the entry grows and later entries shift
	c, _ = c.Update(imagesResolvedMessage{
		imageKey:         7,
		wordReplacements: wordReplacement{"pepeLaugh": strings.Repeat("IMG", 20)},
	})

	require.Greater(t, len(c.lines), before)
	require.Contains(t, strings.Join(c.lines, ""), "IMGIMG")
	require.Zero(t, c.entries[1].Event.imageKey)

	// incremental refresh must match a full re-render
	lines := append([]string(nil), c.lines...)
	positions := make([]position, len(c.entries))
	for i, e := range c.entries {
		positions[i] = e.Position
	}
	cursor := c.cursor

	c.recalculateLines()
	require.Equal(t, lines, c.lines)
	for i, e := range c.entries {
		require.Equal(t, positions[i], e.Position, "entry %d", i)
	}
	require.Equal(t, cursor, c.cursor)

	// still following the newest message
	_, selected := c.entryForCurrentCursor()
	require.Same(t, c.entries[2], selected)
}

func TestRoot_BuildLiveChatEventMessage(t *testing.T) {
	t.Parallel()

	deps := testDeps()
	deps.EmoteReplacer = fakeEmoteReplacer{
		cached:   map[string]string{"pepeLaugh": "text"},
		resolved: map[string]string{"pepeLaugh": "image"},
		pending:  true,
	}
	deps.BadgeReplacer = fakeBadgeReplacer{}
	r := NewUI(nil, deps, nil)

	evt, cmd := r.buildLiveChatEventMessage("acc", testPrivateMessage("1", "pepeLaugh"))
	require.NotNil(t, cmd)
	require.NotZero(t, evt.imageKey)
	require.Equal(t, "text", evt.displayModifier.wordReplacements["pepeLaugh"])

	resolved, ok := cmd().(imagesResolvedMessage)
	require.True(t, ok)
	require.Equal(t, evt.imageKey, resolved.imageKey)
	require.Equal(t, "prepare-emote", resolved.prepareCommand)
	require.Equal(t, "image", resolved.wordReplacements["pepeLaugh"])

	evt2, _ := r.buildLiveChatEventMessage("acc", testPrivateMessage("2", "pepeLaugh"))
	require.NotEqual(t, evt.imageKey, evt2.imageKey)

	// everything cached: no background work
	deps.EmoteReplacer = fakeEmoteReplacer{
		cached:   map[string]string{"pepeLaugh": "image"},
		resolved: map[string]string{"pepeLaugh": "image"},
	}
	evt, cmd = r.buildLiveChatEventMessage("acc", testPrivateMessage("3", "pepeLaugh"))
	require.Nil(t, cmd)
	require.Zero(t, evt.imageKey)
}
