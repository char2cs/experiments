# Quiver Test Minecraft Server

A tiny Minecraft-protocol server built to exercise Quiver end to end. It is not a game server:
it answers server-list pings (so a real Minecraft client shows it in the multiplayer list) and
the UDP query protocol, and politely turns away anyone who tries to join. What it advertises
comes straight from the arrow's variables and ports, which makes every Quiver feature visible
from a client.

| Quiver feature | Where it shows |
|---|---|
| `variables:` | `MOTD`, `MAX_PLAYERS`, `VERSION_NAME` appear in the server list; `LOG_LEVEL` changes what the process logs |
| `netbridge:` | `GAME_PORT` (tcp) and `QUERY_PORT` (udp) are allocated by Quiver and reported back in the status |
| install | downloads the release binary and verifies it against the release's `checksums.txt` |
| execute / stop | runs the server, stops it with a graceful signal |
| methods | `ping` and `query` talk to the running server from the command line |

```arrow
schema: "arrow@v0"

metadata:
  name: "Quiver Test Minecraft Server"
  description: "A Minecraft-protocol test server that proves Quiver variables, netbridge ports and lifecycle end to end."
  license: "MIT"
  url: "https://github.com/char2cs/experiments"
  tags:
    - test
    - minecraft
  maintainers:
    - name: "char2cs"
      url: "https://char2cs.net"

variables:
  - name: MOTD
    type: string
    default: "A Quiver test server"
    description: Message of the day shown in the multiplayer server list. Must not contain double quotes or dollar signs.
  - name: MAX_PLAYERS
    type: number
    default: "20"
    min: 1
    max: 1000
    description: Player slots advertised in the server list.
  - name: VERSION_NAME
    type: string
    default: "Quiver 1.21"
    description: Version label advertised in the server list.
  - name: LOG_LEVEL
    type: select
    default: info
    values: [info, debug]
    description: How chatty the server process is.

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
      cpu_cores: 1
      ram_gb: 1
      disk_gb: 1
    lifecycle:
      install:
        - type: fetch
          title: "Download the test server"
          url:
            linux/amd64: "https://github.com/char2cs/experiments/releases/download/${REF}/mcserver-linux-amd64"
            linux/arm64: "https://github.com/char2cs/experiments/releases/download/${REF}/mcserver-linux-arm64"
            darwin/amd64: "https://github.com/char2cs/experiments/releases/download/${REF}/mcserver-darwin-amd64"
            darwin/arm64: "https://github.com/char2cs/experiments/releases/download/${REF}/mcserver-darwin-arm64"
            "windows/*": "https://github.com/char2cs/experiments/releases/download/${REF}/mcserver-windows-amd64.exe"
          to:
            default: "${WORKDIR}/mcserver"
            "windows/*": "${WORKDIR}\\mcserver.exe"
          checksum: "sha256sums:https://github.com/char2cs/experiments/releases/download/${REF}/checksums.txt"
          timeout: "120s"
          exit_on_failure: true
        - type: run
          title: "Make it executable and check it starts"
          command:
            default: 'chmod +x "${WORKDIR}/mcserver" && "${WORKDIR}/mcserver" version'
            "windows/*": '"${WORKDIR}\mcserver.exe" version'
          timeout: "30s"
          exit_on_failure: true

      execute:
        - type: run
          title: "Run the test server"
          command:
            default: '"${WORKDIR}/mcserver" serve --port ${GAME_PORT} --query-port ${QUERY_PORT} --motd "${MOTD}" --max-players ${MAX_PLAYERS} --version-name "${VERSION_NAME}" --log-level ${LOG_LEVEL}'
            "windows/*": '"${WORKDIR}\mcserver.exe" serve --port ${GAME_PORT} --query-port ${QUERY_PORT} --motd "${MOTD}" --max-players ${MAX_PLAYERS} --version-name "${VERSION_NAME}" --log-level ${LOG_LEVEL}'

      stop:
        - type: signal
          signal: graceful
          timeout: "30s"
          exit_on_failure: false

      uninstall:
        - type: run
          title: "Remove the test server"
          command:
            default: 'rm -f "${WORKDIR}/mcserver"'
            "windows/*": 'del /f /q "${WORKDIR}\mcserver.exe"'
          timeout: "30s"
          exit_on_failure: false

    methods:
      ping:
        available_in: [running]
        steps:
          - type: run
            title: "Ping the server like a Minecraft client"
            command:
              default: '"${WORKDIR}/mcserver" ping --port ${GAME_PORT}'
              "windows/*": '"${WORKDIR}\mcserver.exe" ping --port ${GAME_PORT}'
            timeout: "15s"
      query:
        available_in: [running]
        steps:
          - type: run
            title: "Query the server over UDP"
            command:
              default: '"${WORKDIR}/mcserver" query --port ${QUERY_PORT}'
              "windows/*": '"${WORKDIR}\mcserver.exe" query --port ${QUERY_PORT}'
            timeout: "15s"
```
