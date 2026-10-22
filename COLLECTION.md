# experiments

Arrows for exercising [quiver.core](https://github.com/rabbytesoftware/quiver.core) end to end.

| Arrow | What |
|---|---|
| `minecraft` | the real vanilla Minecraft server: variables, netbridge ports and a dependency on `java` |
| `java` | Eclipse Temurin 21 JRE, with a `java` export other arrows launch |

```collection
schema: "collection@v0"

metadata:
  name: "Quiver Experiments"
  description: "A real Minecraft server arrow and the Java runtime it depends on."
  url: "https://github.com/char2cs/experiments"
  maintainers:
    - "char2cs"
  tags:
    - testing
    - minecraft

arrows:
  - path: arrows/minecraft
  - path: arrows/java
```
