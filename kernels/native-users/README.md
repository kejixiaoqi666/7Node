# Optional native user-update kernel

This directory builds `xbord-native-users`, a small external Go protocol kernel
for the experimental Rust controller. It supports the controller's current
single VLESS TCP / Trojan TCP inbound, optional file TLS, direct outbound and
system hostname resolution. It does not include all sing-box protocols,
services or the original Go node's management features.

中文说明及本轮实测见 [用户热更新报告](../../docs/RUST_NATIVE_USERS_ZH.md)。

## Why a companion?

The default controller mode runs a stock sing-box process. A user-list change
restarts that process, interrupting existing connections. This opt-in companion
can atomically replace its authentication snapshot while the listener and
already authenticated connections keep running. Listener/protocol/TLS changes
still require the controller's validated restart and recovery path.

Stable identities come from each user's name, not their array position. An
update rejects duplicate names, duplicate credentials and invalid data before
publishing a new snapshot. Removing a user denies subsequent authentication;
already authenticated connections may continue. It is not an immediate
disconnect/revocation mechanism.

## Build on Linux

Requires Python 3 and Go 1.26.8. Go's pinned module downloads need network access
on the first build. `--test-race` additionally needs a working C compiler.
Use a fresh, absolute work path each time:

```bash
python3 kernels/native-users/build.py \
  --go /absolute/path/to/go \
  --work /absolute/path/to/new-native-build \
  --test-race
```

The script checks six upstream/replacement SHA-256 pairs, uses private copied
dependencies, runs tests (and optional race tests), then builds a trimmed binary
with `CGO_ENABLED=0`. The generated project and dependency sources remain in the
work directory. Running `go build` directly in this directory omits those
patches and is not the supported build procedure.

The Linux delivery also includes a vendored corresponding-source archive with
an offline build procedure and dependency licenses. See its README for the
path-only module metadata adjustments and exact source manifest.

## Use with the Rust controller

Use [runtime-rust-native.json](../../examples/runtime-rust-native.json): set
`native_user_updates` to `true` and point `singbox_executable` at the built
`xbord-native-users` binary. Keep the state directory short enough for a Unix
socket path and private to the runtime user. Newly created state directories
are 0700; candidate files and sockets are 0600. No public control TCP port is
opened.

```bash
cargo build -p node-runtime --release --locked
./target/release/xboard-node-rust --config examples/runtime-rust-native.json --check
read -rsp 'Panel token: ' XBORD_PANEL_TOKEN
printf '\n'
export XBORD_PANEL_TOKEN
./target/release/xboard-node-rust --config examples/runtime-rust-native.json
```

Edit the example's URL, IDs and paths before running. `--check` only validates
runtime settings; it does not contact a panel or prove authentication works.
The panel token is excluded from the protocol kernel's configuration and
environment. Stop with Ctrl+C or SIGTERM.

Native mode requires Unix and has been exercised on Linux ARM64. It is rejected
on Windows. Omitting the option or setting it to `false` retains stock-process
behavior; supply stock sing-box for that mode. Enabling the option against an
unmodified stock binary fails the required capability handshake.

## Transaction and failure behavior

Each candidate remains limited to 16 MiB of encoded configuration. Before
activation the controller runs a bounded `check`, then requests replacement
using the previous and next SHA-256 digests. The companion compares all
non-user configuration, checks the expected digest and validates the entire
user snapshot before a single atomic publication.

Rust limits a control call to two seconds and limits reply frames to 8 KiB.
After a missing/invalid reply it makes a separate bounded status call:

- New digest: commit the candidate without restarting the kernel.
- Old digest: keep the last successful snapshot and retry on a later sync.
- Unknown state: terminate/reap the uncertain kernel and recover from the last
  successful snapshot. This recovery interrupts its connections.

The successful runtime snapshot currently lives in memory. A whole controller
restart still needs the panel. Accounting, limits, device/IP enforcement,
other protocols, custom routing/DNS, multiple nodes and persistent recovery
remain migration work.

## Reproduce acceptance

```bash
python3 benchmarks/native_users_lab.py \
  --controller /absolute/path/to/xboard-node-rust \
  --native /absolute/path/to/xbord-native-users \
  --singbox /absolute/path/to/official-sing-box \
  --domain your-valid-certificate-hostname.example \
  --certificate-dir /absolute/path/to/certificate-directory \
  --output /absolute/short/path/to/fresh-lab
```

This creates isolated loopback listeners, a synthetic HTTPS panel and official
sing-box clients. It reads the supplied `fullchain.pem` / `privkey.pem` in place
and never includes them in its output. The directory must be fresh; a short
path is needed for Unix sockets. `--only-faults` and `--only-lifecycle` run
bounded subsets. Resource sampling scans `/proc` then waits 10 ms; peaks are
sampled RSS, not proof of every instantaneous allocation.

## License

The companion is GPL-3.0-or-later with retained upstream terms/notices. See
[NOTICE.md](NOTICE.md), [LICENSE](LICENSE) and the two upstream license files.
The separate Rust controller retains MPL-2.0. This companion is not an
official sing-box release.
