package save

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestKeyMap_MarshalYAML(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		Up: key.NewBinding(key.WithKeys("w", "q"), key.WithHelp("w", "test")),
	}

	doc, err := yaml.Marshal(keyMap)

	require.NoError(t, err)
	require.Equal(t, "up:\n    - w\n    - q\n", string(doc))

}

func TestKeyMap_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	gotKeyMap := &KeyMap{
		Up: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "test-help")),
	}

	err := yaml.Unmarshal([]byte("up:\n    - w\n    - q\n"), &gotKeyMap)
	require.NoError(t, err)
	require.Equal(t, []string{"w", "q"}, gotKeyMap.Up.Keys())
	require.Equal(t, "test-help", gotKeyMap.Up.Help().Desc)   // should not be overwritten
	require.Equal(t, []string{"w", "q"}, gotKeyMap.Up.Keys()) // should be overwritten

}

func TestKeyMap_InsertModeAcceptsBothCases(t *testing.T) {
	t.Parallel()

	keyMap := BuildDefaultKeyMap()

	// Verify that InsertMode binding includes both lowercase and uppercase 'i'
	keys := keyMap.InsertMode.Keys()
	require.Contains(t, keys, "i", "InsertMode should accept lowercase 'i'")
	require.Contains(t, keys, "I", "InsertMode should accept uppercase 'I' (for Caps Lock)")
}
