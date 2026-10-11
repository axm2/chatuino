# Settings

Chatuino can run without a settings file, but you may want to configure its default behavior.

Your settings file is read from `~/.config/chatuino/settings.yaml` (the config directory may differ depending on your OS). Create the file if it doesn't exist.

```yaml
vertical_tab_list: false # Display tabs vertically instead of horizontally
moderation:
  store_chat_logs: true # Store chat logs in a SQLite database; Default: false

  # NOTE: logs_channel_include and logs_channel_exclude are mutually exclusive.
  logs_channel_include: ["lirik", "sodapoppin"] # Only log specified channels
  # logs_channel_exclude: ["lec"] # Log all channels except those specified

security:
  check_links: true # Check and display HTTP redirects next to URLs. Uses Chatuino server to hide IP when resolving; Default: true

# Globally block specific users and words
block_settings:
  users:
    - julezdev
  words:
    - Kappa
chat:
  # NOTE: Read the README for more information about emote rendering before enabling this feature
  graphic_emotes: true # Display emotes as images instead of text; Default: false
  graphic_badges: true # Display badges as images instead of text; Default: false
  emote_animation: auto # How animated emotes are played: auto, native, client or off; Default: auto
  disable_badges: false # Hide badges entirely; Default: false
  smooth_scroll: true # Animate chat scrolling when new messages arrive; Default: false
  time_format: "15:04:05" # Go time format for message timestamps; Default: "15:04:05"
custom_commands:
  # Custom commands are available as command suggestions
  - trigger: "/ocean"
    replacement: "OCEAN MAN 🌊 😍 Take me by the hand ✋ lead me to the land that you understand 🙌 🌊 OCEAN MAN 🌊 😍 The voyage 🚲 to the corner of the 🌎 globe is a real trip 👌 🌊 OCEAN MAN 🌊 😍 The crust of a tan man 👳 imbibed by the sand 👍 Soaking up the 💦 thirst of the land 💯"
```

## Time Format

The `time_format` setting uses Go's reference time format. Go uses a specific reference time (`Mon Jan 2 15:04:05 MST 2006`) to define formats. You construct your desired format by showing how this reference time should be displayed.

Common format examples:

| Format String | Example Output | Description |
| ------------- | -------------- | ----------- |
| `15:04:05` | `14:30:45` | 24-hour with seconds (default) |
| `15:04` | `14:30` | 24-hour without seconds |
| `3:04 PM` | `2:30 PM` | 12-hour with AM/PM |
| `3:04:05 PM` | `2:30:45 PM` | 12-hour with seconds and AM/PM |
| `Jan 2 15:04` | `Dec 25 14:30` | Month, day, and time |
| `2006-01-02 15:04` | `2024-12-25 14:30` | ISO date and time |

You can use [this Go Playground link](https://go.dev/play/p/mgHrQLytjZM) to prototype and test custom time formats.

Reference components:
- Year: `2006`
- Month: `01` or `Jan`
- Day: `02`
- Hour (24h): `15`
- Hour (12h): `3`
- Minute: `04`
- Second: `05`
- AM/PM: `PM`

## NO_COLOR

Chatuino respects the `NO_COLOR` environment variable and will not render colors if enabled.

```sh
NO_COLOR=1 chatuino
```

## Key Bindings

Key bindings are configurable in the `~/.config/chatuino/keymap.yaml` file (the config directory may differ depending on your OS).

Press `?` inside Chatuino to view an overview of available key bindings.

## Custom Commands

The settings allow you to configure custom commands which will be suggested to you during text input.

```yaml
custom_commands:
  - trigger: "/ocean"
    replacement: "OCEAN MAN 🌊 😍 Take me by the hand ✋ lead me to the land that you understand 🙌 🌊 OCEAN MAN 🌊 😍 The voyage 🚲 to the corner of the 🌎 globe is a real trip 👌 🌊 OCEAN MAN 🌊 😍 The crust of a tan man 👳 imbibed by the sand 👍 Soaking up the 💦 thirst of the land 💯"
```

You can also create templated dynamic commands:

```yaml
custom_commands:
  - trigger: "/with-template"
    replacement: "CurrentTime: {{ .CurrentTime }}; CurrentDateTime: {{ .CurrentDateTime }}; BroadcastID: {{ .BroadcastID }}; BroadcastName: {{ .BroadcastName }}; SelectedDisplayName: {{ .SelectedDisplayName }}; SelectedUserID: {{ .SelectedUserID }}; Message: {{ .SelectedMessageContent }} "
    # You can even create custom mod commands/shortcuts
  - trigger: "/custom-spam-timeout"
    replacement: "/timeout {{ .SelectedDisplayName }} 10 Please stop spamming."
```

All features of [Go's templating engine](https://pkg.go.dev/text/template) are available.

Available template data:

| Name | Situation | Description |
| ---- | --------- | ----------- |
| CurrentTime | Any | The current local time |
| CurrentDateTime | Any | The current local date time |
| BroadcastID | Any | The broadcaster ID |
| BroadcastName | Any | The broadcaster name |
| SelectedDisplayName | Chat Message Selected | The senders display name |
| SelectedUserID | Chat Message Selected | The senders user ID |
| SelectedMessageContent | Chat Message Selected | The senders message |
| SubMessageCumulativeMonths | Sub Alert Message Selected | Chatters subbed months (cumulative) |
| SubMessageStreakMonths | Sub Alert Message Selected | Chatters subbed streak |
| SubMessageSubPlan | Sub Alert Message Selected | Chatters sub tier |
| SubGiftReceiptDisplayName | Sub Gift Message Selected | Gift receipt display name |
| SubGiftRecipientID | Sub Gift Message Selected | Gift receipt user id |
| SubGiftMonths | Sub Gift Message Selected | Gift months |
| SubGiftSubPlan | Sub Gift Message Selected | Gift sub tier |
| SubGiftGiftMonths | Sub Gift Message Selected | Gift months |
| RawMessage | Any | The complete internal message struct |
| MessageType | Any | Type of message (PrivateMessage, SubMessage, SubGiftMessage) |

The data is inserted when you accept the suggestion (default: Tab), allowing you to review the text before sending.

You can use templating at any time when composing a message. The data is inserted when you accept the suggestion.

> **Note**: Templates are never rendered when you press Enter. Always review your input before sending.

## Emote Support

### Text Emotes

When `graphic_emotes` is disabled, Chatuino displays emotes as colored text. The color depends on the emote provider:

| Color | Provider |
| ----- | -------- |
| <span style="color:#0aa6ec">Blue</span> | SevenTV |
| <span style="color:#a35df2">Purple</span> | Twitch |
| <span style="color:#d50014">Red</span> | BetterTTV |
| <span style="color:#a3be8c">Green</span> | FFZ |

### Graphic Emotes

Chatuino can display images and animated images as Twitch emotes using the Kitty Graphics Protocol. This protocol is implemented by Kitty and some other terminals. However, it uses the [Unicode placeholder method](https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders), which is currently only fully implemented by Kitty. It also works with Ghostty, see [Animated Emotes](#animated-emotes).

Currently, this feature is **only** available in Kitty and Ghostty terminals on Unix platforms. This may change in the future.

#### Animated Emotes

Kitty plays animated emotes itself. Ghostty accepts the animation commands but does not play them, so in Ghostty Chatuino swaps the frames itself. The `emote_animation` setting controls this:

| Value | Behavior |
| ----- | -------- |
| `auto` | `native` in Kitty, `client` in all other terminals (default) |
| `native` | All frames are sent to the terminal, which plays the animation |
| `client` | Chatuino swaps frames itself. Only emotes currently on screen are animated |
| `off` | Animated emotes show their first frame only |

With `client`, each emote is updated at most 15 times per second and at most 600 frame updates are sent per second in total. With many animated emotes on screen, updates become less frequent, but playback speed stays the same. In Ghostty this costs roughly 1.5% CPU per 100 frame updates per second.

#### Format Support and Caching

Chatuino is statically compiled without dynamic library dependencies, allowing it to run on any system without additional requirements. Emote format support prioritizes native Go decoding for performance and stability.

Chatuino prefers formats that can be decoded natively in Go:
- **PNG** (static images) - Native Go support
- **GIF** (animated images) - Native Go support
- **WebP** (static images) - Native Go support
- **AVIF** and **animated WebP** - Decoded via WASM (libavif/libwebp) when necessary

The WASM-based decoders may consume more memory but are only used as a fallback. Chatuino caches all decoded images, so each emote is decoded only once per session.

Emotes are cached in the `~/.local/share/chatuino/emote` directory using the Kitty image transmission format, compressed with RFC 1950 ZLIB deflate compression.

Query the current cache size:

```sh
chatuino cache
```

Delete cached data:

```sh
chatuino cache clear --emotes --database --badges
```
