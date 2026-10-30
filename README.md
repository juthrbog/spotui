# spotui

spotui is a Spotify player for the terminal. It plays music through the speakers of your computer. You do not need the Spotify desktop app.

spotui contains [go-librespot](https://github.com/devgianlu/go-librespot), an open-source implementation of the Spotify Connect protocol. Spotify does not support this protocol for third-party apps. A change on the Spotify side can stop playback until go-librespot gets an update.

spotui also shows up as a Spotify Connect device. You can control it from the Spotify app on your phone.

## Requirements

You need these items to use spotui:

- A Spotify Premium account. Spotify does not allow playback in third-party players without Premium.
- One of these systems:
  - Linux on x86-64 or ARM64, with PulseAudio or PipeWire. ALSA also works.
  - macOS 12 or later on Apple Silicon.
  - Windows 10 or later on x64. Use Windows Terminal.

## Install

### Homebrew (macOS and Linux)

```sh
brew install juthrbog/tap/spotui
```

### Download a release

Each [release](https://github.com/juthrbog/spotui/releases) has these files:

| File | System |
| --- | --- |
| `spotui_<version>_linux_amd64.tar.gz` | Linux on x86-64 |
| `spotui_<version>_linux_arm64.tar.gz` | Linux on ARM64 |
| `spotui_<version>_darwin_arm64.tar.gz` | macOS on Apple Silicon |
| `spotui_<version>_windows_amd64.zip` | Windows on x64 |

The Linux binary needs glibc 2.28 or later and the ALSA library (`alsa-lib` on Arch Linux, `libasound2` on Debian and Ubuntu). Most desktop distributions have both. The macOS and Windows binaries need nothing else.

These commands download version 0.1.0 for Linux on x86-64 and install it in `~/.local/bin`. Change `version` and `platform` as necessary:

```sh
version=0.1.0 platform=linux_amd64
curl -LO "https://github.com/juthrbog/spotui/releases/download/v${version}/spotui_${version}_${platform}.tar.gz"
gh attestation verify "spotui_${version}_${platform}.tar.gz" --repo juthrbog/spotui
tar -xzf "spotui_${version}_${platform}.tar.gz" spotui
install -D -m 755 spotui ~/.local/bin/spotui
```

The `gh attestation verify` command is optional. It makes sure that the GitHub release workflow of this repository built the file. Each release also has a `checksums.txt` file.

On Windows, extract `spotui.exe` from the zip file and run it in Windows Terminal.

### Build from source

To build spotui, you need these items:

- Go 1.27.2 or later.
- A C compiler, `pkg-config`, and the libraries for Ogg, Vorbis, FLAC, and mpg123. Linux also needs the ALSA library.

On Arch Linux, this command installs the libraries:

```sh
sudo pacman -S --needed base-devel pkgconf alsa-lib libogg libvorbis flac mpg123
```

On macOS, this command installs them:

```sh
brew install pkgconf libogg libvorbis flac mpg123
```

On Windows, [MSYS2](https://www.msys2.org/) supplies the C compiler, `pkg-config`, and the libraries. Do these steps:

1. Install MSYS2 in `C:\msys64`.
2. Open the MSYS2 UCRT64 shell and run this command:

   ```sh
   pacman -S --needed mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-pkgconf mingw-w64-ucrt-x86_64-libogg mingw-w64-ucrt-x86_64-libvorbis mingw-w64-ucrt-x86_64-flac mingw-w64-ucrt-x86_64-mpg123
   ```

3. Add `C:\msys64\ucrt64\bin` to the `PATH` environment variable of Windows. Go then finds `gcc` and `pkg-config` in PowerShell too.
4. Open a new PowerShell window, so that it uses the new `PATH`.
5. In this PowerShell window, set these environment variables. Then run the `go install` or `go build` command below in the same window.

   ```powershell
   $env:CGO_CFLAGS = "-O2 -g -DFLAC__NO_DLL"
   $env:CGO_LDFLAGS = "-static"
   $env:GOFLAGS = "-ldflags=-extldflags=-Wl,-logg,-lshlwapi"
   ```

These variables link the libraries statically into `spotui.exe`. The binary then needs no DLL files from MSYS2, so it also runs on other computers. A dynamic link fails, because the libogg DLL of MSYS2 does not export `ogg_stream_iovecin`. `-DFLAC__NO_DLL` tells the FLAC headers that FLAC is a static library. The `GOFLAGS` value adds libogg again at the end of the link, after libvorbis, and adds `shlwapi` for libmpg123.

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
| `A` / `I` | Go to the album or the artist of the track |
| `esc` | Go back, or clear the filter |
| `1` / `2` / `tab` | Library tab or Search tab |
| `f` | Filter the list on screen |
| `/` | Search all of Spotify |
| `R` | Load the library again |
| `L` | Log in to the Web API |
| `?` | Show all keys |
| `q` | Quit |

To filter a list, push `f` and type. The list shows only the items that contain every word that you type, for example `beatles abbey`. The filter works in the Library tab and in each track list. It uses no Web API calls. Push `enter` to keep the filter and go to the list, or push `esc` to clear it.

`A` and `I` apply to the selected track. If no track is selected, they apply to the song that plays. If the track has more than one artist, `I` shows a list of the artists. Select one and push `enter`.

To search, push `/`, type the search, and push `enter`. While you type, the keys go into the search box. To stop typing, push `esc` or `tab`. In the results, push `esc` to type again.

## Cover art

If the terminal is 90 columns or wider, spotui shows a panel on the right. The panel shows the cover of the selected playlist, album, artist, or song. If nothing is selected, the panel shows the cover of the song that plays.

spotui draws covers in one of three ways:

- Kitty graphics show sharp images. kitty and Ghostty support them. In this mode, spotui also shows a small cover next to the song that plays.
- Sixel graphics show sharp images. Windows Terminal 1.22 and later support them. In this mode, spotui shows only the cover in the panel.
- Half blocks draw each cover with colored `▀` characters. They work in all terminals with true color, but the image is not sharp.

With the default `cover_art = "auto"`, spotui asks the terminal at start which graphics it supports. If the terminal supports kitty graphics, spotui uses them. On Windows, spotui then uses sixel graphics if the terminal supports them. In all other cases, spotui uses half blocks.

spotui was tested with sixel graphics only in Windows Terminal. On Linux and macOS, `auto` does not select sixel graphics. To use them in a terminal that supports them, such as foot or WezTerm, set `cover_art = "sixel"`.

### Terminal multiplexers

A terminal multiplexer is a program that shows several terminal sessions in one window, such as herdr or tmux. When spotui runs in a terminal multiplexer, covers can be less sharp. When spotui asks which graphics the terminal supports, the multiplexer answers, not the terminal. If the multiplexer does not pass graphics through to the terminal, spotui uses half blocks.

For example, herdr does not pass sixel graphics through to Windows Terminal. For sharp covers in Windows Terminal, run spotui directly in a tab of Windows Terminal, not in herdr. herdr has an experimental `kitty_graphics` option, but it works only when the terminal that runs herdr supports kitty graphics.

If a cover shows as an empty box or as strange characters, set `cover_art = "blocks"` in the configuration file.

## Configuration

spotui writes the configuration file `config.toml` with comments on its first start. The [Files](#files) section gives its location. The file contains these values:

| Key | Default | Meaning |
| --- | --- | --- |
| `client_id` | empty | Client ID of your Spotify developer app |
| `redirect_port` | `8898` | Port of the login callback for the Web API |
| `device_name` | `spotui` | Name in the Spotify Connect device list |
| `audio_backend` | depends on the system | Linux: `pulseaudio` (also for PipeWire) or `alsa`. macOS: `audio-toolbox` or `pulseaudio`. Windows: `wasapi`. The first value is the default. |
| `audio_device` | empty | Output device. Empty means the system default. |
| `bitrate` | `320` | Stream quality in kbit/s: `96`, `160`, or `320` |
| `initial_volume` | `50` | Volume on the first start, from 0 to 100 |
| `cover_art` | `auto` | `auto`, `kitty`, `sixel`, `blocks`, or `off` |

If you change `redirect_port`, change the redirect URI of your developer app to the same port.

## Files

spotui keeps its files in two directories:

| System | Configuration file | Other files |
| --- | --- | --- |
| Linux | `~/.config/spotui/` | `~/.local/state/spotui/` |
| macOS | `~/Library/Application Support/spotui/` | `~/Library/Application Support/spotui/` |
| Windows | `%AppData%\spotui\` | `%LocalAppData%\spotui\` |

The files are:

- `config.toml`: the configuration file.
- `player-state.json`: the Spotify login and the device ID.
- `webapi-token.json`: the Web API login.
- `spotui.log`: the log.

The login files contain secrets. On Linux and macOS, spotui makes them readable only by your user.

To remove both logins, run this command:

```sh
spotui -logout
```

## Troubleshooting

If you hear no sound on Linux, set `audio_backend = "alsa"` in the configuration file and restart spotui.

If something fails, read the log `spotui.log`. The [Files](#files) section gives its location. For more detail, start spotui with `spotui -debug`.

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

The `Taskfile.yml` file has the common tasks, for example `task test` and `task lint`. Run `task --list` to see all tasks. `task lint` needs golangci-lint 2.14.0, the version that CI uses.

The workflows pin each GitHub Action to a commit SHA, golangci-lint to a version, and govulncheck as a tool in `go.mod`. Dependabot proposes updates for the Go modules and the GitHub Actions every week, after a cooldown of 7 days. Security updates do not wait.

To make a release, do these steps:

1. Merge the changes into the `main` branch.
2. On the `main` branch, run `task release -- v0.1.0` with the new version. The task makes the tag and pushes it.
3. The Release workflow tests the code and builds the binaries for Linux (`amd64`, `arm64`), macOS (`arm64`), and Windows (`amd64`). It runs each binary on a computer of the same type. Then it publishes the release with the archives, `checksums.txt`, and build attestations, and updates the formula in the [Homebrew tap](https://github.com/juthrbog/homebrew-tap).

All release binaries link the audio libraries statically. The Linux binaries are built in a Debian 10 container, so they run on most distributions. The Windows binary is built with MinGW in a container, and the macOS binary on a Mac.

To build a release binary on your computer, run `task release:build -- <target>`. The targets are `linux-amd64`, `linux-arm64`, and `windows-amd64`, which need Docker, and `darwin-arm64`, which needs a Mac. The binary goes to `dist/<os>_<arch>/`.

The tap update needs the `HOMEBREW_TAP_SSH_KEY` secret in this repository. It is the private key of a deploy key with write access to the tap. Prerelease tags, such as `v0.2.0-rc.1`, do not update the tap.

## License

spotui is free software under the GNU General Public License, version 3. Refer to [LICENSE](LICENSE). spotui contains go-librespot, which uses the same license.

spotui is not affiliated with Spotify. Spotify is a trademark of Spotify AB.
