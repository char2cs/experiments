# experiments

A Quiver **collection** for exercising [quiver.core](https://github.com/rabbytesoftware/quiver.core)
end to end, for demos and as a regression harness.

| Arrow | What |
|---|---|
| `minecraft` | the real vanilla Minecraft Java server (Mojang's `server.jar`), configured by Quiver variables and ports |
| `java` | Eclipse Temurin 21 JRE, exporting `java`; `minecraft` depends on it through `tools:` |
| `mcserver` | a tiny Minecraft-protocol stand-in (Go) for CI: no Java, no 60 MB download, starts instantly |

```bash
quiver install github.com/char2cs/experiments/minecraft   # installs java first, then the server
quiver run github.com/char2cs/experiments/minecraft \
  --data ACCEPT_EULA=true --data MOTD="Hello from Quiver" --data MAX_PLAYERS=33
quiver github.com/char2cs/experiments/minecraft address
quiver stop github.com/char2cs/experiments/minecraft
```

Add the server in Minecraft with the allocated `GAME_PORT` (`quiver status`).

**EULA.** The server will not start unless you pass `--data ACCEPT_EULA=true`, which means you
accept the [Minecraft EULA](https://aka.ms/MinecraftEULA).

## What each Quiver feature shows

| Feature | In `minecraft` |
|---|---|
| `variables:` | `MOTD`, `MAX_PLAYERS`, `DIFFICULTY` (select), `GAME_MODE` (select), `ONLINE_MODE` (select), `MEMORY_MB` (`-Xmx`), `ACCEPT_EULA` (boolean gate) become `server.properties` |
| `netbridge:` | `GAME_PORT` (tcp) is `server-port`, `QUERY_PORT` (udp) is `query.port` |
| `tools:` | `github.com/char2cs/experiments/java@v0.*`; the server is launched with `${github.com/char2cs/experiments/java.java}` |
| install | `server.jar` fetched from Mojang and verified against a pinned SHA-256 |
| execute / stop | writes the config, runs the server; SIGTERM makes it save the world and exit |
| `methods:` | `address` prints where to connect |

The `java` arrow pins Temurin archives with their SHA-256s and exports a different path per platform
(macOS archives nest the binary under `Contents/Home`). The dependency is declared as `@v0.*`, so a
release tagged `v0.x.y` of this repo is what a dependent resolves to, with no manifest edit per tag.

## Limits

- `MOTD` is placed inside double quotes in a shell command: keep `"`, `$` and backticks out of it.
- The Windows commands of `minecraft` and `java` are written but have not been run.
- Versions are pinned (Minecraft 1.21.11, Temurin 21.0.x). To bump them, update the URL and checksum
  in the manifest; `make e2e-real` fails loudly with the actual hash if a pinned checksum is stale.
- `mcserver` downloads from this repo's GitHub releases: push a tag `vX.Y.Z` and the `release`
  workflow builds every platform plus `checksums.txt`.

## Layout

| Path | What |
|---|---|
| `COLLECTION.md` | the collection manifest |
| `arrows/minecraft.md`, `arrows/java.md`, `arrows/mcserver.md` | the arrows (the fenced `arrow` block is what Quiver reads) |
| `cmd/mcserver`, `internal/mcproto`, `internal/server` | the stand-in server and the Minecraft protocol client both servers are tested with |
| `e2e` | manifest checks and the end-to-end tests |

## Tests

```bash
make test         # unit tests + manifest checks, with the race detector
make e2e          # mcserver: renders the manifest's own commands and runs them
make e2e-real     # minecraft: the same against Mojang's real server (java + network, ~15 s)
make e2e-daemon QUIVER_BIN=/path/to/quiver   # drives a real quiver.core daemon
```

- **Manifest checks** (all arrows): defaults valid, every `${NAME}` resolves (Quiver leaves a typo
  verbatim, so it would reach the shell), every dependency reference names a declared tool and an
  export that arrow really has, nothing declared but unused, lifecycle pairing, timeouts, https and
  checksums on every download, the collection lists exactly the arrow files.
- **Process e2e**: takes the commands from the manifests, substitutes variables and free ports the way
  Quiver does, runs them through the shell and talks to the server with a Minecraft client
  (`internal/mcproto`). For `minecraft` this covers the EULA gate, `server.properties`, the real
  status and query replies and the SIGTERM save.
- **Daemon e2e**: a real `quiver` against an isolated home. `mcserver` uses a local HTTP server for its
  release; `minecraft` uses the real downloads (`E2E_REAL=1`), so the `java` dependency is installed
  by Quiver from Temurin.

### Known quiver.core gap the daemon e2e reports

At quiver.core `bad7799` the runtime repository builds its assembler with a `nil` Netbridge
(`internal/app/repositories/runtime/runtime.go`), so `netbridge:` ports are never allocated and
`${GAME_PORT}` reaches the shell unresolved. The `netbridge allocates the declared ports` stage fails
with that message until core wires Netbridge in. Everything before it (seed, install with checksum,
dependency install) passes. Wiring it in locally shows a second failure at `run`
(`begin execution: asynx: validation failed`), not yet diagnosed.
