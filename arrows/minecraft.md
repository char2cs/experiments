# Minecraft Server (vanilla 1.21.11)

The real Mojang Minecraft Java server, packaged as an arrow so Quiver can be shown end to end:
variables become `server.properties`, Quiver-allocated ports become the game and query ports,
and the Java runtime comes from a sibling arrow in this collection.

| Quiver feature | Where it shows |
|---|---|
| `variables:` | `MOTD`, `MAX_PLAYERS`, `DIFFICULTY`, `GAME_MODE`, `ONLINE_MODE` land in `server.properties`; `MEMORY_MB` becomes `-Xmx` |
| `netbridge:` | `GAME_PORT` (tcp) is the game port, `QUERY_PORT` (udp) the query port |
| `tools:` | the `java` arrow of this collection is installed first and its `java` export launches the server |
| install | downloads Mojang's `server.jar` and verifies its SHA-256 |
| execute / stop | writes the config, runs the server, stops it with a graceful signal so the world is saved |

**EULA.** Running this arrow means accepting the [Minecraft EULA](https://aka.ms/MinecraftEULA).
It will not start unless you say so explicitly: `--data ACCEPT_EULA=true`.

**Limits.** `MOTD` is placed inside double quotes in a shell command: keep `"`, `$` and backticks
out of it. The Windows commands are written but have not been run.

```arrow
schema: "arrow@v0"

metadata:
  name: "Minecraft Server"
  description: "The vanilla Minecraft Java server, configured by Quiver variables and ports."
  url: "https://github.com/char2cs/experiments"
  tags:
    - minecraft
    - game-server
  maintainers:
    - name: "char2cs"
      url: "https://char2cs.net"
  credits:
    - name: "Mojang Studios"
      url: "https://www.minecraft.net"

variables:
  - name: ACCEPT_EULA
    type: boolean
    description: Set to true to accept the Minecraft EULA (https://aka.ms/MinecraftEULA). Asked for on every start; the server will not run otherwise.
  - name: MOTD
    type: string
    default: "A Quiver Minecraft server"
    description: Message of the day. Must not contain double quotes, dollar signs or backticks.
  - name: MAX_PLAYERS
    type: number
    default: "20"
    min: 1
    max: 1000
    description: Player slots.
  - name: DIFFICULTY
    type: select
    default: easy
    values: [peaceful, easy, normal, hard]
    description: World difficulty.
  - name: GAME_MODE
    type: select
    default: survival
    values: [survival, creative, adventure, spectator]
    description: Default game mode.
  - name: ONLINE_MODE
    type: select
    default: "true"
    values: ["true", "false"]
    description: Verify players with Mojang. Turn off for offline LAN demos.
  - name: MEMORY_MB
    type: number
    default: "1024"
    min: 512
    max: 16384
    description: Maximum Java heap, in megabytes.

netbridge:
  - name: GAME_PORT
    protocol: tcp
    default: 25565
    required: true
  - name: QUERY_PORT
    protocol: udp
    default: 25566
    required: true

targets:
  "*":
    requirements:
      cpu_cores: 2
      ram_gb: 2
      disk_gb: 2
    tools:
      - github.com/char2cs/experiments/java@main
    lifecycle:
      install:
        - type: fetch
          title: "Download the Minecraft server"
          url: "https://piston-data.mojang.com/v1/objects/64bb6d763bed0a9f1d632ec347938594144943ed/server.jar"
          to:
            default: "${WORKDIR}/server.jar"
            "windows/*": "${WORKDIR}\\server.jar"
          checksum: "sha256:f83b8e093865806f931c7e34aae41b177d4c076335263dd124c75d6d65dd1726"
          timeout: "300s"
          exit_on_failure: true

      execute:
        - type: run
          title: "Write server.properties"
          command:
            default: |-
              [ "${ACCEPT_EULA}" = "true" ] || { echo "ACCEPT_EULA must be true: run with --data ACCEPT_EULA=true to accept the Minecraft EULA (https://aka.ms/MinecraftEULA)" >&2; exit 1; }; echo eula=true > "${WORKDIR}/eula.txt" && printf '%s\n' "server-port=${GAME_PORT}" "enable-query=true" "query.port=${QUERY_PORT}" "motd=${MOTD}" "max-players=${MAX_PLAYERS}" "difficulty=${DIFFICULTY}" "gamemode=${GAME_MODE}" "online-mode=${ONLINE_MODE}" > "${WORKDIR}/server.properties"
            "windows/*": |-
              powershell -NoProfile -Command "if ('${ACCEPT_EULA}' -ne 'true') { Write-Error 'ACCEPT_EULA must be true: accept the Minecraft EULA (https://aka.ms/MinecraftEULA)'; exit 1 }; Set-Content -Path '${WORKDIR}\eula.txt' -Value 'eula=true'; Set-Content -Path '${WORKDIR}\server.properties' -Value @('server-port=${GAME_PORT}', 'enable-query=true', 'query.port=${QUERY_PORT}', 'motd=${MOTD}', 'max-players=${MAX_PLAYERS}', 'difficulty=${DIFFICULTY}', 'gamemode=${GAME_MODE}', 'online-mode=${ONLINE_MODE}')"
          timeout: "30s"
          exit_on_failure: true
        - type: run
          title: "Run the Minecraft server"
          command:
            default: 'cd "${WORKDIR}" && "${github.com/char2cs/experiments/java.java}" -Xmx${MEMORY_MB}M -jar server.jar nogui'
            "windows/*": 'cd /d "${WORKDIR}" && "${github.com/char2cs/experiments/java.java}" -Xmx${MEMORY_MB}M -jar server.jar nogui'

      stop:
        - type: signal
          signal: graceful
          timeout: "60s"
          exit_on_failure: false

      uninstall:
        - type: run
          title: "Remove the server files"
          command:
            default: 'rm -rf "${WORKDIR}/server.jar" "${WORKDIR}/eula.txt" "${WORKDIR}/server.properties"'
            "windows/*": 'cmd /c "del /f /q ${WORKDIR}\\server.jar ${WORKDIR}\\eula.txt ${WORKDIR}\\server.properties"'
          timeout: "30s"
          exit_on_failure: false

    methods:
      address:
        available_in: [running]
        steps:
          - type: run
            title: "Show where to connect"
            command:
              default: 'echo "Add this server in Minecraft: localhost:${GAME_PORT}"'
              "windows/*": 'echo Add this server in Minecraft: localhost:${GAME_PORT}'
            timeout: "10s"
```
