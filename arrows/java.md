# Temurin JRE 21

The Java runtime the Minecraft server arrow runs on, packaged as its own arrow so any arrow in
this collection (or anywhere else) can depend on it with `tools:` and reach the binary through
the `java` export instead of guessing where it was unpacked.

Archives and checksums are pinned to the Eclipse Temurin 21 releases published on GitHub. Windows
on ARM runs the x64 build under emulation.

```arrow
schema: "arrow@v0"

metadata:
  name: "Temurin JRE 21"
  description: "Eclipse Temurin 21 Java runtime, exported for arrows that run on the JVM."
  license: "GPL-2.0-with-classpath-exception"
  url: "https://adoptium.net"
  tags:
    - java
    - runtime
  maintainers:
    - name: "char2cs"
      url: "https://char2cs.net"
  credits:
    - name: "Eclipse Adoptium"
      url: "https://adoptium.net"

targets:
  "*":
    requirements:
      cpu_cores: 1
      ram_gb: 1
      disk_gb: 1
    exports:
      java:
        linux/amd64: ./jdk-21.0.13+9-jre/bin/java
        linux/arm64: ./jdk-21.0.12.1+1-jre/bin/java
        darwin/amd64: ./jdk-21.0.12.1+1-jre/Contents/Home/bin/java
        darwin/arm64: ./jdk-21.0.13+9-jre/Contents/Home/bin/java
        "windows/*": ./jdk-21.0.12.1+1-jre/bin/java.exe
    lifecycle:
      install:
        - type: fetch
          title: "Download the Temurin JRE"
          url:
            linux/amd64: "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.13%2B9/OpenJDK21U-jre_x64_linux_hotspot_21.0.13_9.tar.gz"
            linux/arm64: "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.12.1%2B1/OpenJDK21U-jre_aarch64_linux_hotspot_21.0.12.1_1.tar.gz"
            darwin/amd64: "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.12.1%2B1/OpenJDK21U-jre_x64_mac_hotspot_21.0.12.1_1.tar.gz"
            darwin/arm64: "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.13%2B9/OpenJDK21U-jre_aarch64_mac_hotspot_21.0.13_9.tar.gz"
            "windows/*": "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.12.1%2B1/OpenJDK21U-jre_x64_windows_hotspot_21.0.12.1_1.zip"
          to:
            linux/amd64: "${WORKDIR}/jre.tar.gz"
            linux/arm64: "${WORKDIR}/jre.tar.gz"
            darwin/amd64: "${WORKDIR}/jre.tar.gz"
            darwin/arm64: "${WORKDIR}/jre.tar.gz"
            "windows/*": "${WORKDIR}\\jre.zip"
          checksum:
            linux/amd64: "a535a1df9c6284d9618872984e026ae0352d1f7e9e0605425c0e6114f47351dd"
            linux/arm64: "14be1f35ebdbd1f6e8d57eb911a3ffb74d6d9aa255abc5daf2b1302002cf2cf2"
            darwin/amd64: "6717ec641fd9ce0bb209ca083ee23b42202ac68cb6fcc5753496e0e4a0f41989"
            darwin/arm64: "225bab0a09491ef06835488cd942f5d2befa9a0410a2484f0c8bbcf7506ec09a"
            "windows/*": "d35f31e712f0fcf6ac5a093edc90204fbff22f720ba3950bd09d331d5e621636"
          timeout: "300s"
          exit_on_failure: true
        - type: extract
          title: "Unpack the JRE"
          from:
            linux/amd64: "./jre.tar.gz"
            linux/arm64: "./jre.tar.gz"
            darwin/amd64: "./jre.tar.gz"
            darwin/arm64: "./jre.tar.gz"
            "windows/*": "./jre.zip"
          to: "./"
          timeout: "120s"
          exit_on_failure: true
        - type: run
          title: "Check the runtime starts"
          command:
            linux/amd64: '"${WORKDIR}/jdk-21.0.13+9-jre/bin/java" -version'
            linux/arm64: '"${WORKDIR}/jdk-21.0.12.1+1-jre/bin/java" -version'
            darwin/amd64: '"${WORKDIR}/jdk-21.0.12.1+1-jre/Contents/Home/bin/java" -version'
            darwin/arm64: '"${WORKDIR}/jdk-21.0.13+9-jre/Contents/Home/bin/java" -version'
            "windows/*": '"${WORKDIR}\jdk-21.0.12.1+1-jre\bin\java.exe" -version'
          timeout: "60s"
          exit_on_failure: true
      uninstall:
        - type: run
          title: "Remove the JRE"
          command:
            default: 'rm -rf "${WORKDIR}"/jdk-* "${WORKDIR}"/jre.tar.gz'
            "windows/*": 'cmd /c "rmdir /s /q ${WORKDIR}\jdk-21.0.12.1+1-jre & del /f /q ${WORKDIR}\jre.zip"'
          timeout: "60s"
          exit_on_failure: false
```
