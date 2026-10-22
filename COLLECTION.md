# experiments

Arrows for exercising [quiver.core](https://github.com/rabbytesoftware/quiver.core) end to end.

| Arrow | What |
|---|---|
| `minecraft` | the real vanilla Minecraft server: variables, netbridge ports and a dependency on `java` |
| `java` | Eclipse Temurin 21 JRE, with a `java` export other arrows launch |
| `mcserver` | a tiny Minecraft-protocol stand-in used by CI: no Java, no 60 MB download, starts instantly |

```collection
schema: "collection@v0"

metadata:
  name: "Quiver Experiments"
  description: "A real Minecraft server arrow, the Java runtime it depends on, and a fast stand-in for CI."
  url: "https://github.com/char2cs/experiments"
  maintainers:
    - "char2cs"
  tags:
    - testing
    - minecraft

arrows:
  - path: arrows/minecraft
  - path: arrows/java
  - path: arrows/mcserver
```
