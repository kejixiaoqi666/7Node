# Native user kernel: source and license notices

`xbord-native-users` is an optional, separate Go executable. It is a modified
build using the sing-box ecosystem, not an official sing-box release and not a
pure Rust protocol implementation. No upstream affiliation or endorsement is
implied.

The new companion code is GPL-3.0-or-later. Its license is in `LICENSE`.
The repository's Rust controller remains under its existing MPL-2.0 license;
that declaration does not relicense the companion's upstream code.

Pinned upstream inputs are recorded in `patches/manifest.json`:

- `github.com/cedar2025/sing-box`, commit `2e665cb7e295`, version
  `v1.14.0-alpha.2.0.20260316103356-2e665cb7e295`.
- `github.com/sagernet/sing-vmess`, commit `3aed155119a1`, version
  `v0.2.8-0.20250909125414-3aed155119a1`.

Their original notices and license terms are retained in
`UPSTREAM-LICENSE-sing-box` and `UPSTREAM-LICENSE-sing-vmess`, including the
sing-box upstream additional name/association terms. Other dependency notices
are retained in the vendored corresponding-source archive distributed with the
Linux companion binary.

The original upstream notices are retained in the license files listed above.
Six source files were modified on 2026-10-02 to replace mutable credential maps with immutable atomic
snapshots, use stable authenticated string identities instead of user-list
positions, validate complete updates before publication, and read fragmented
Trojan credentials with `io.ReadFull`. The companion adds a bounded, private
Unix socket control interface with candidate digest reconciliation.

`build.py` verifies exact upstream and replacement file hashes, copies
dependencies into a fresh private build directory, and applies the replacements
there. It never patches a shared module cache or the repository's original Go
runtime. The normal root Go build and installation scripts are unchanged.
