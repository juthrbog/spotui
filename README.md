# spotui

spotui is a Spotify player for the terminal. It plays music through the speakers of your computer. You do not need the Spotify desktop app.

spotui contains [go-librespot](https://github.com/devgianlu/go-librespot), an open-source implementation of the Spotify Connect protocol. Spotify does not support this protocol for third-party apps. A change on the Spotify side can stop playback until go-librespot gets an update.

spotui also shows up as a Spotify Connect device. You can control it from the Spotify app on your phone.

## Requirements

You need these items to use spotui:

- A Spotify Premium account. Spotify does not allow playback in third-party players without Premium.
- Linux on x86-64 or ARM64, with PulseAudio or PipeWire. ALSA also works.

## Install

### Download a release

Each [release](https://github.com/juthrbog/spotui/releases) has a binary for Linux on x86-64 (`amd64`) and on ARM64 (`arm64`). The binary needs glibc 2.28 or later and the ALSA library (`alsa-lib` on Arch Linux, `libasound2` on Debian and Ubuntu). Most desktop distributions have both.

These commands download version 0.1.0 for x86-64 and install it in `~/.local/bin`. Change `version` and `arch` as necessary:

```sh
version=0.1.0 arch=amd64
curl -LO "https://github.com/juthrbog/spotui/releases/download/v${version}/spotui_${version}_linux_${arch}.tar.gz"
gh attestation verify "spotui_${version}_linux_${arch}.tar.gz" --repo juthrbog/spotui
tar -xzf "spotui_${version}_linux_${arch}.tar.gz" spotui
install -D -m 755 spotui ~/.local/bin/spotui
```

The `gh attestation verify` command is optional. It makes sure that the GitHub release workflow of this repository built the file. Each release also has a `checksums.txt` file.

### Build from source

To build spotui, you need these items:

- Go 1.27.2 or later.
- A C compiler, `pkg-config`, and the libraries for ALSA, Ogg, Vorbis, FLAC, and mpg123.

On Arch Linux, this command installs the libraries:

```sh
sudo pacman -S --needed base-devel pkgconf alsa-lib libogg libvorbis flac mpg123
```

To install the latest version from source, run this command:

```sh
go install github.com/juthrbog/spotui@latest
```

To build from a copy of the repository, run these commands:

```sh
git clone https://github.com/juthrbog/spotui.git
cd spotui
go build -o spotui .
```

To show the version of spotui, run `spotui -version`.

## First start

1. Run `spotui`. If you built from the source code, run `./spotui` in the `spotui` directory.
2. spotui opens a Spotify login page in your browser. If the browser does not open, open the link that spotui shows.
3. Log in to Spotify and approve the device. If Spotify asks for a code, enter the code that spotui shows.

spotui saves the login. You log in only one time.

After the login, the Library tab shows Liked Songs and your playlists. Select a playlist and push enter to see its tracks. Select a track and push enter to play it.

## Set up search and saved albums

Search and saved albums use the Spotify Web API. The Web API needs a client ID from your own Spotify developer app. A client ID is the public name of an app on the Spotify platform. Playback and playlists work without it.

1. Open the [Spotify developer dashboard](https://developer.spotify.com/dashboard) and log in.
2. Create an app. The name and description can be any text.
3. Add this redirect URI: `http://127.0.0.1:8898/callback`
4. Under "Which API/SDKs are you planning to use?", select Web API.
5. Save the app and copy its client ID.
6. Write the client ID into `client_id` in `~/.config/spotui/config.toml`.
7. Restart spotui.

When spotui starts, it opens a second login page in your browser. Approve it. spotui saves this login too.

You can also give the client ID in the `SPOTUI_CLIENT_ID` environment variable. The variable overrides the value in the configuration file.

Spotify sets these limits on apps in development mode:

- The owner of the app must have Spotify Premium.
- Five Spotify users at most can use one app.
- A search gives 10 results at most for each type.

## Keys

| Key | Action |
| --- | --- |
| `space` | Play or pause |
| `n` / `p` | Next or previous track |
| `←` / `→` | Seek back or forward 5 seconds |
| `+` / `-` | Volume up or down |
| `s` | Shuffle on or off |
| `r` | Repeat: off, all, one |
| `↑` `↓` or `j` `k` | Move the selection |
| `g` / `G` | First or last item |
| `pgup` / `pgdown` | Page up or down |
| `enter` | Open the selected item, or play it |
| `esc` | Go back, or clear the filter |
| `1` / `2` / `tab` | Library tab or Search tab |
| `f` | Filter the list on screen |
| `/` | Search all of Spotify |
| `R` | Load the library again |
| `L` | Log in to the Web API |
| `?` | Show all keys |
| `q` | Quit |

To filter a list, push `f` and type. The list shows only the items that contain every word that you type, for example `beatles abbey`. The filter works in the Library tab and in each track list. It uses no Web API calls. Push `enter` to keep the filter and go to the list, or push `esc` to clear it.

To search, push `/`, type the search, and push `enter`. While you type, the keys go into the search box. To stop typing, push `esc` or `tab`. In the results, push `esc` to type again.

## Cover art

If the terminal is 90 columns or wider, spotui shows a panel on the right. The panel shows the cover of the selected playlist, album, artist, or song. If nothing is selected, the panel shows the cover of the song that plays.

spotui draws covers in one of two ways:

- Kitty graphics show sharp images. kitty and Ghostty support them. In this mode, spotui also shows a small cover next to the song that plays.
- Half blocks draw each cover with colored `▀` characters. They work in all terminals with true color, but the image is not sharp.

With the default `cover_art = "auto"`, spotui asks the terminal at start if it supports kitty graphics. If the terminal does not answer, spotui uses half blocks.

If a cover shows as an empty box or as strange characters, set `cover_art = "blocks"` in the configuration file. This can happen when spotui runs inside a program that manages the terminal, such as herdr. In herdr, you can also turn on `kitty_graphics` in the herdr configuration.

## Configuration

spotui writes `~/.config/spotui/config.toml` with comments on its first start. The file contains these values:

| Key | Default | Meaning |
| --- | --- | --- |
| `client_id` | empty | Client ID of your Spotify developer app |
| `redirect_port` | `8898` | Port of the login callback for the Web API |
| `device_name` | `spotui` | Name in the Spotify Connect device list |
| `audio_backend` | `pulseaudio` | `pulseaudio` (also for PipeWire) or `alsa` |
| `audio_device` | empty | Output device. Empty means the system default. |
| `bitrate` | `320` | Stream quality in kbit/s: `96`, `160`, or `320` |
| `initial_volume` | `50` | Volume on the first start, from 0 to 100 |
| `cover_art` | `auto` | `auto`, `kitty`, `blocks`, or `off` |

If you change `redirect_port`, change the redirect URI of your developer app to the same port.

## Files

spotui keeps its files in these locations:

- `~/.config/spotui/config.toml`: the configuration file.
- `~/.local/state/spotui/player-state.json`: the Spotify login and the device ID.
- `~/.local/state/spotui/webapi-token.json`: the Web API login.
- `~/.local/state/spotui/spotui.log`: the log.

The login files contain secrets. spotui makes them readable only by your user.

To remove both logins, run this command:

```sh
spotui -logout
```

## Troubleshooting

If you hear no sound, set `audio_backend = "alsa"` in the configuration file and restart spotui.

If something fails, read the log at `~/.local/state/spotui/spotui.log`. For more detail, start spotui with `spotui -debug`.

If the Web API login fails with an error about the redirect URI, make sure that the redirect URI in your developer app is exactly `http://127.0.0.1:8898/callback`. Spotify does not accept `localhost` in redirect URIs.

Run one copy of spotui at a time. Two copies use the same device ID, and Spotify cannot tell them apart.

## Limits

- spotui loads 500 saved albums at most.
- A track list shows 800 tracks at most.
- Search does not find playlists that Spotify made, such as Discover Weekly. spotui shows them only if they are in your library.

## How it works

spotui runs the go-librespot player inside its own process. The player plays the audio and keeps the queue. The terminal interface sends commands to the player over Go channels.

Liked Songs, playlists, and track lists come from the internal Spotify API through go-librespot. Search and saved albums come from the public Web API through your developer app. Spotify limits the Web API by app, so a personal app gives you your own limit.

## Development

The `Taskfile.yml` file has the common tasks, for example `task test` and `task lint`. Run `task --list` to see all tasks.

To make a release, do these steps:

1. Merge the changes into the `main` branch.
2. On the `main` branch, run `task release -- v0.1.0` with the new version. The task makes the tag and pushes it.
3. The Release workflow tests the code and builds the binaries for `amd64` and `arm64`. It runs each binary on a computer of the same type. Then it publishes the release with the archives, `checksums.txt`, and build attestations.

The release binaries are built in a Debian 10 container. The container links the audio libraries statically, so the binaries run on most distributions. To build a release binary on your computer, run `task release:build -- amd64`. The build needs Docker and writes the binary to `dist/linux_amd64/spotui`.

## License

spotui is free software under the GNU General Public License, version 3. Refer to [LICENSE](LICENSE). spotui contains go-librespot, which uses the same license.

spotui is not affiliated with Spotify. Spotify is a trademark of Spotify AB.
