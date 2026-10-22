# experiments

A Quiver collection for exercising [quiver.core](https://github.com/rabbytesoftware/quiver.core)
end to end: a real Minecraft server and the Java runtime it depends on.

| Arrow | What |
|---|---|
| `minecraft` | the vanilla Minecraft Java server (Mojang's `server.jar`), configured by Quiver variables and ports |
| `java` | Eclipse Temurin 21 JRE, exporting `java`; `minecraft` depends on it through `tools:` |

```bash
quiver install github.com/char2cs/experiments/minecraft   # installs java first, then the server
quiver run github.com/char2cs/experiments/minecraft \
  --data ACCEPT_EULA=true --data MOTD="Hello from Quiver" --data MAX_PLAYERS=33
quiver github.com/char2cs/experiments/minecraft address
quiver stop github.com/char2cs/experiments/minecraft
```

Add the server in Minecraft with the allocated `GAME_PORT` (`quiver status`).

**EULA.** The server will not start unless you pass `--data ACCEPT_EULA=true`, which means you accept
the [Minecraft EULA](https://aka.ms/MinecraftEULA).

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
(macOS archives nest the binary under `Contents/Home`). The dependency is declared as `@v0.*`, so the
repo must have a tag `v0.x.y` for a dependent to resolve it.

## Limits

- `MOTD` is placed inside double quotes in a shell command: keep `"`, `$` and backticks out of it.
- The Windows commands are written but have not been run.
- Versions are pinned (Minecraft 1.21.11, Temurin 21.0.x). To bump them, update the URL and checksum.

## Verified so far

Seed, install (including the automatic `java` dependency install, with real downloads and checksums)
and the server's own commands (EULA gate, `server.properties`, status/query replies, SIGTERM save) were
run against a real `quiver.core` and a real Minecraft server.

At quiver.core `bad7799` the runtime repository builds its assembler with a `nil` Netbridge
(`internal/app/repositories/runtime/runtime.go`), so `netbridge:` ports are never allocated and
`${GAME_PORT}` reaches the shell unresolved. `quiver run` will not work for `minecraft` until core
wires Netbridge in.
