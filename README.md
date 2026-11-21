# ChatLoom Instagram bridge

This is [mautrix-meta](https://github.com/mautrix/meta) (the `mautrix-instagram`
binary) with one addition used by [ChatLoom](https://github.com/siudajakub/chatloom):
an authenticated provisioning endpoint, `POST /v3/chatloom/link_media`, that
resolves Instagram post, reel and story links through your own Instagram login,
so ChatLoom can play shared Instagram media natively. Requests are bounded
(strict URL parsing, two at a time, 256 MiB per video source and 64 MiB per output,
both also respecting the configured media-size limit) and nothing
is cached by the bridge.

Shared videos are converted before reaching the phone: H.264 MP4, up to
720×1280 (or 1280×720), 30 fps, CRF 28, 1.8 Mbit/s video maximum rate and
96 kbit/s AAC audio. Smaller sources are never enlarged. MP4 faststart allows
playback before the complete file is read. The bridge selects the smallest
Instagram variant that can supply this resolution instead of the largest one.
This applies to permalink downloads, new shared reel/story/post uploads and
XMA direct-media downloads. Ordinary videos sent in a conversation retain their
existing behavior. Files already uploaded to Matrix retain their original size.

Runtime requirements: `ffmpeg` on PATH with `libx264` and `aac` encoders
(included in the Docker images). Conversion runs one at a time, with two codec
threads and a 60-second conversion budget. The permalink endpoint allows
70 seconds for downloading and conversion, within ChatLoom's 75-second timeout;
older JSON callers remain compatible. A missing encoder, busy converter,
timeout, or oversized result fails explicitly; the bridge never falls back to
sending the original video. For busy permalink requests it returns HTTP 429.

Set `CHATLOOM_MEDIA_TMPDIR` (or `TMPDIR`) to a private directory on disk, with
at least 320 MiB free for one source and output. Use disk-backed storage so
temporary video files do not consume RAM.
Each conversion uses a separate private directory and deletes its source when
encoding completes, then deletes the output when its reader closes. These
temporary files are not a reusable cache. Install the updated bridge binary to
enable compression; updating the Flutter client alone cannot change a running
bridge.

Targeted tests: `go test ./pkg/msgconv/mediadl ./pkg/msgconv/igconv ./pkg/igconnector`.
Set `CHATLOOM_TEST_FFMPEG=1` to also verify encoding using short synthetic videos
and `ffprobe` (no Instagram requests). Point `TMPDIR` and Go caches to disk
before running tests on the devbox.

The default branch `chatloom/link-media` is the original build, on upstream
`782b06b` (2026-09-19). `chatloom/update-2026-10-25` carries the same endpoint
merged with upstream `main` at `5b6a5b3` (2026-10-25), which includes upstream's
switch of story fetching to POST. Build either
with `./build-ig.sh` and run it with Beeper's `bbctl` like the official bridge; see
[Self-hosting ChatLoom](https://github.com/siudajakub/chatloom/blob/main/docs/SELF-HOSTING.md).
Messenger is unchanged, so use the official `mautrix-meta` for it.
The upstream README follows.

---

# mautrix-meta
A Matrix-Facebook Messenger and Instagram DM puppeting bridge.

This repo contains two bridges: mautrix-meta for Facebook Messenger and
mautrix-instagram for Instagram DMs. When setting up the Instagram bridge,
note the differences in setup instructions (different build script, different
docker tags, etc).

## Documentation
All setup and usage instructions are located on [docs.mau.fi]. Some quick links:

[docs.mau.fi]: https://docs.mau.fi/bridges/go/meta/index.html

* Facebook [Bridge setup](https://docs.mau.fi/bridges/go/setup.html?bridge=meta)
  (or [with Docker](https://docs.mau.fi/bridges/general/docker-setup.html?bridge=meta))
* Instagram [Bridge setup](https://docs.mau.fi/bridges/go/setup.html?bridge=instagram)
  (or [with Docker](https://docs.mau.fi/bridges/general/docker-setup.html?bridge=instagram))
* Basic usage: [Authentication](https://docs.mau.fi/bridges/go/meta/authentication.html)

### Features & Roadmap
[ROADMAP.md](ROADMAP.md) contains a general overview of what is supported by the bridge.

## Discussion
Matrix room: [#meta:maunium.net](https://matrix.to/#/#meta:maunium.net)
