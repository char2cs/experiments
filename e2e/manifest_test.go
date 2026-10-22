package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const collectionNS = "github.com/char2cs/experiments"

var platforms = []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"}

func load(t *testing.T, name string) *Manifest {
	t.Helper()
	m, err := LoadArrow(name)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func eachArrow(t *testing.T, fn func(t *testing.T, name string, m *Manifest)) {
	t.Helper()
	for _, name := range Arrows {
		t.Run(name, func(t *testing.T) { fn(t, name, load(t, name)) })
	}
}

func TestCollection_ListsEveryArrowFile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "COLLECTION.md"))
	if err != nil {
		t.Fatal(err)
	}
	found := fence.FindSubmatch(raw)
	if found == nil || string(found[1]) != "collection" {
		t.Fatal("COLLECTION.md has no collection block")
	}
	var c struct {
		Schema   string `yaml:"schema"`
		Metadata struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		} `yaml:"metadata"`
		Arrows []struct {
			Path string `yaml:"path"`
		} `yaml:"arrows"`
	}
	if err := yaml.Unmarshal(found[2], &c); err != nil {
		t.Fatal(err)
	}
	if c.Schema != "collection@v0" || c.Metadata.Name == "" || c.Metadata.Description == "" {
		t.Fatalf("incomplete collection %+v", c)
	}
	listed := map[string]bool{}
	for _, a := range c.Arrows {
		listed[strings.TrimPrefix(a.Path, "arrows/")] = true
		if _, err := os.Stat(filepath.Join("..", a.Path+".md")); err != nil {
			t.Errorf("collection entry %s has no file: %v", a.Path, err)
		}
	}
	for _, name := range Arrows {
		if !listed[name] {
			t.Errorf("arrow %s is not in COLLECTION.md", name)
		}
	}
	if len(listed) != len(Arrows) {
		t.Errorf("collection lists %d arrows, tests know %d", len(listed), len(Arrows))
	}
}

func TestManifest_SchemaAndNames(t *testing.T) {
	eachArrow(t, func(t *testing.T, _ string, m *Manifest) {
		if m.Schema != "arrow@v0" || m.Metadata.Name == "" {
			t.Fatalf("schema %q name %q", m.Schema, m.Metadata.Name)
		}
	})
}

func TestManifest_VariableDefaultsAreValid(t *testing.T) {
	eachArrow(t, func(t *testing.T, _ string, m *Manifest) {
		for _, v := range m.Variables {
			switch v.Type {
			case "number":
				n, err := strconv.Atoi(v.Default)
				if err != nil {
					t.Errorf("%s: default %q is not a number", v.Name, v.Default)
				} else if n < v.Min || n > v.Max {
					t.Errorf("%s: default %d outside [%d,%d]", v.Name, n, v.Min, v.Max)
				}
			case "select":
				if !contains(v.Values, v.Default) {
					t.Errorf("%s: default %q not in %v", v.Name, v.Default, v.Values)
				}
			case "boolean":
				if v.Default != "true" && v.Default != "false" {
					t.Errorf("%s: default %q is not a boolean", v.Name, v.Default)
				}
			}
		}
	})
}

func TestManifest_PortsAreRequiredAndDistinct(t *testing.T) {
	eachArrow(t, func(t *testing.T, _ string, m *Manifest) {
		seen := map[int]string{}
		for _, p := range m.Netbridge {
			if !p.Required {
				t.Errorf("port %s must be required so a failed allocation aborts the run", p.Name)
			}
			if p.Default < 1 || p.Default > 65535 {
				t.Errorf("%s: default %d out of range", p.Name, p.Default)
			}
			if other, dup := seen[p.Default]; dup {
				t.Errorf("%s and %s share default %d", p.Name, other, p.Default)
			}
			seen[p.Default] = p.Name
		}
	})
}

// Quiver leaves an unresolved ${NAME} verbatim, so a typo would reach the
// shell. Every token must be a built-in, a variable, a port or an export of a
// declared tool.
func TestManifest_EveryReferenceResolves(t *testing.T) {
	builtIns := map[string]bool{"WORKDIR": true, "INSTALL_PATH": true, "REF": true, "PLATFORM": true, "ARROW_NAMESPACE": true}
	eachArrow(t, func(t *testing.T, _ string, m *Manifest) {
		known := map[string]bool{}
		for k := range builtIns {
			known[k] = true
		}
		for _, v := range m.Variables {
			known[v.Name] = true
		}
		for _, p := range m.Netbridge {
			known[p.Name] = true
		}
		used := map[string]bool{}
		for key, target := range m.Targets {
			for _, step := range allSteps(target) {
				for _, text := range stepTexts(step) {
					for _, name := range References(text) {
						used[name] = true
						if strings.Contains(name, "/") {
							checkDependencyReference(t, target, name)
							continue
						}
						if !known[name] {
							t.Errorf("target %s: step %q references unknown ${%s}", key, step.Title, name)
						}
					}
				}
			}
		}
		for _, v := range m.Variables {
			if !used[v.Name] {
				t.Errorf("variable %s is declared but never used", v.Name)
			}
		}
		for _, p := range m.Netbridge {
			if !used[p.Name] {
				t.Errorf("port %s is declared but never used", p.Name)
			}
		}
	})
}

func checkDependencyReference(t *testing.T, target Target, name string) {
	t.Helper()
	i := strings.LastIndex(name, ".")
	if i < 0 {
		t.Errorf("dependency reference ${%s} has no export name", name)
		return
	}
	ns, export := name[:i], name[i+1:]
	var declared bool
	for _, tool := range target.Tools {
		bare, _, _ := strings.Cut(tool, "@")
		if bare == ns {
			declared = true
		}
	}
	if !declared {
		t.Errorf("${%s} names %s, which is not in tools:", name, ns)
		return
	}
	if !strings.HasPrefix(ns, collectionNS+"/") {
		return
	}
	dep := load(t, strings.TrimPrefix(ns, collectionNS+"/"))
	for _, dt := range dep.Targets {
		if _, ok := dt.Exports[export]; !ok {
			t.Errorf("${%s}: %s does not export %q", name, ns, export)
		}
	}
}

func TestManifest_LifecycleIsComplete(t *testing.T) {
	eachArrow(t, func(t *testing.T, name string, m *Manifest) {
		for key, target := range m.Targets {
			lc := target.Lifecycle
			if len(lc["install"]) == 0 || len(lc["uninstall"]) == 0 {
				t.Errorf("target %s: install and uninstall must both be present", key)
			}
			if len(lc["execute"]) > 0 {
				for _, step := range lc["stop"] {
					if step.Type != "signal" || step.Signal != "graceful" {
						t.Errorf("stop step %+v should be a graceful signal", step)
					}
				}
				if len(lc["stop"]) == 0 {
					t.Errorf("target %s: execute without stop", key)
				}
			} else if len(lc["stop"]) > 0 {
				t.Errorf("target %s: stop without execute", key)
			}
			timeout := regexp.MustCompile(`^\d+[sm]$`)
			for hook, steps := range lc {
				for _, step := range steps {
					if step.Timeout != "" && !timeout.MatchString(step.Timeout) {
						t.Errorf("%s step %q: timeout %q must match ^\\d+[sm]$", hook, step.Title, step.Timeout)
					}
				}
			}
		}
	})
}

func TestManifest_DownloadsAreHTTPSAndVerified(t *testing.T) {
	eachArrow(t, func(t *testing.T, _ string, m *Manifest) {
		for _, target := range m.Targets {
			for _, step := range target.Lifecycle["install"] {
				if step.Type != "fetch" {
					continue
				}
				for _, p := range platforms {
					url, ok := Overrideable(step.URL, p)
					if !ok || !strings.HasPrefix(url, "https://") {
						t.Errorf("%s: url %q must be https", p, url)
					}
					sum, ok := Overrideable(step.Checksum, p)
					if !ok || sum == "" {
						t.Errorf("%s: fetch has no checksum", p)
					}
					if to, ok := Overrideable(step.To, p); !ok || !strings.HasPrefix(to, "${WORKDIR}") {
						t.Errorf("%s: destination %q must be workdir-anchored", p, to)
					}
				}
			}
		}
	})
}

func TestMinecraft_ConfigAndDependency(t *testing.T) {
	m := load(t, "minecraft")
	target := m.Targets["*"]

	vars := map[string]string{}
	for _, v := range m.Variables {
		vars[v.Name] = v.Default
	}
	if vars["ACCEPT_EULA"] != "false" {
		t.Errorf("ACCEPT_EULA must default to false, got %q", vars["ACCEPT_EULA"])
	}

	if len(target.Tools) != 1 || !strings.HasPrefix(target.Tools[0], collectionNS+"/java@") {
		t.Fatalf("tools %v: want the collection's java arrow with an explicit selector", target.Tools)
	}

	cfg, _ := Overrideable(target.Lifecycle["execute"][0].Command, "linux/amd64")
	for _, want := range []string{"${ACCEPT_EULA}", "server-port=${GAME_PORT}", "query.port=${QUERY_PORT}", "motd=${MOTD}",
		"max-players=${MAX_PLAYERS}", "difficulty=${DIFFICULTY}", "gamemode=${GAME_MODE}", "online-mode=${ONLINE_MODE}", "enable-query=true"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config step does not write %s", want)
		}
	}
	for _, p := range platforms {
		run, _ := Overrideable(target.Lifecycle["execute"][1].Command, p)
		if !strings.Contains(run, "${github.com/char2cs/experiments/java.java}") || !strings.Contains(run, "-Xmx${MEMORY_MB}M") {
			t.Errorf("%s: server is not launched with the java export and heap size: %q", p, run)
		}
	}
	if m := target.Methods["address"]; len(m.AvailableIn) != 1 || m.AvailableIn[0] != "running" {
		t.Errorf("address method %+v", m)
	}
}

func TestJava_ExportsEveryPlatform(t *testing.T) {
	m := load(t, "java")
	target := m.Targets["*"]
	exp, ok := target.Exports["java"]
	if !ok {
		t.Fatal("java arrow exports no java")
	}
	for _, p := range platforms {
		path, ok := Overrideable(exp, p)
		if !ok || !strings.HasPrefix(path, "./") || strings.Contains(path, "${") {
			t.Errorf("%s: export %q must be a static ./ path", p, path)
		}
		if strings.HasPrefix(p, "windows") != strings.HasSuffix(path, ".exe") {
			t.Errorf("%s: export %q has the wrong extension", p, path)
		}
		if strings.HasPrefix(p, "darwin") != strings.Contains(path, "/Contents/Home/") {
			t.Errorf("%s: export %q has the wrong layout", p, path)
		}
	}
	steps := target.Lifecycle["install"]
	if len(steps) != 3 || steps[0].Type != "fetch" || steps[1].Type != "extract" || steps[2].Type != "run" {
		t.Fatalf("install steps %+v", steps)
	}
	// The archive is fetched, unpacked and probed at the same place the export points to.
	for _, p := range platforms {
		export, _ := Overrideable(exp, p)
		probe, _ := Overrideable(steps[2].Command, p)
		if !strings.Contains(strings.ReplaceAll(probe, `\`, "/"), strings.TrimPrefix(export, "./")) {
			t.Errorf("%s: probe %q does not run the exported %q", p, probe, export)
		}
	}
}

func TestMcserver_DeclaresTheContract(t *testing.T) {
	m := load(t, "mcserver")
	vars := map[string]string{}
	for _, v := range m.Variables {
		vars[v.Name] = v.Type
	}
	for name, typ := range map[string]string{"MOTD": "string", "MAX_PLAYERS": "number", "VERSION_NAME": "string", "LOG_LEVEL": "select"} {
		if vars[name] != typ {
			t.Errorf("variable %s: want type %s, got %q", name, typ, vars[name])
		}
	}
	ports := map[string]string{}
	for _, p := range m.Netbridge {
		ports[p.Name] = p.Protocol
	}
	if ports["GAME_PORT"] != "tcp" || ports["QUERY_PORT"] != "udp" {
		t.Fatalf("ports %v", ports)
	}
	for _, name := range []string{"ping", "query"} {
		method, ok := m.Targets["*"].Methods[name]
		if !ok || len(method.AvailableIn) != 1 || method.AvailableIn[0] != "running" {
			t.Errorf("method %s: %+v", name, method)
		}
	}
	fetch := m.Targets["*"].Lifecycle["install"][0]
	if sum, _ := Overrideable(fetch.Checksum, "linux/amd64"); !strings.HasPrefix(sum, "sha256sums:") {
		t.Errorf("checksum %q should point at the release checksums", sum)
	}
	for _, p := range platforms {
		if url, _ := Overrideable(fetch.URL, p); !strings.Contains(url, "${REF}") {
			t.Errorf("%s: url %q must carry ${REF}", p, url)
		}
	}
}

func TestOverrideable_Resolution(t *testing.T) {
	fetch := load(t, "mcserver").Targets["*"].Lifecycle["install"][0]
	testCases := []struct {
		platform string
		suffix   string
	}{
		{"linux/amd64", "mcserver-linux-amd64"},
		{"darwin/arm64", "mcserver-darwin-arm64"},
		{"windows/arm64", "mcserver-windows-amd64.exe"},
	}
	for _, tc := range testCases {
		t.Run(tc.platform, func(t *testing.T) {
			got, ok := Overrideable(fetch.URL, tc.platform)
			if !ok || !strings.HasSuffix(got, tc.suffix) {
				t.Fatalf("got %q ok=%v", got, ok)
			}
		})
	}
	if _, ok := Overrideable(fetch.URL, "plan9/amd64"); ok {
		t.Fatal("unmapped platform must not resolve")
	}
	if _, ok := Overrideable(yaml.Node{Kind: yaml.SequenceNode}, "linux/amd64"); ok {
		t.Fatal("a sequence is not an overrideable scalar")
	}
}

func TestParse_Errors(t *testing.T) {
	if _, _, err := Parse([]byte("# nothing here")); err == nil {
		t.Fatal("missing block should fail")
	}
	if _, _, err := Parse([]byte("```arrow\n: : :\n```")); err == nil {
		t.Fatal("invalid yaml should fail")
	}
	if _, err := LoadArrow("nope"); err == nil {
		t.Fatal("missing arrow should fail")
	}
}

func TestRender_LeavesUnresolvedVerbatim(t *testing.T) {
	got := Render("a ${X} b ${Y} $HOME ${ns/a.b}", map[string]string{"X": "1", "ns/a.b": "2"})
	if got != "a 1 b ${Y} $HOME 2" {
		t.Fatalf("got %q", got)
	}
}

func allSteps(t Target) []Step {
	var out []Step
	for _, steps := range t.Lifecycle {
		out = append(out, steps...)
	}
	for _, m := range t.Methods {
		out = append(out, m.Steps...)
	}
	return out
}

func stepTexts(s Step) []string {
	var out []string
	for _, n := range []yaml.Node{s.Command, s.URL, s.To, s.From, s.Checksum} {
		out = append(out, nodeValues(n)...)
	}
	return out
}

func nodeValues(n yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.MappingNode:
		var out []string
		for i := 1; i < len(n.Content); i += 2 {
			out = append(out, n.Content[i].Value)
		}
		return out
	default:
		return nil
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
