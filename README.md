# ChatLoom Instagram bridge

This is [mautrix-meta](https://github.com/mautrix/meta) (the `mautrix-instagram`
binary) with one addition used by [ChatLoom](https://github.com/siudajakub/chatloom):
an authenticated provisioning endpoint, `POST /v3/chatloom/link_media`, that
resolves Instagram post, reel and story links through your own Instagram login,
so ChatLoom can play shared Instagram media natively. Requests are bounded
(strict URL parsing, two at a time, 64 MiB per file) and nothing is cached by
the bridge.

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
