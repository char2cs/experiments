// Package e2e holds the checks that prove the arrow manifest and the server it
// ships agree with each other and with Quiver.
package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest is the part of arrow@v0 this repository's checks look at.
type Manifest struct {
	Schema   string `yaml:"schema"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Variables []struct {
		Name    string   `yaml:"name"`
		Type    string   `yaml:"type"`
		Default string   `yaml:"default"`
		Values  []string `yaml:"values"`
		Min     int      `yaml:"min"`
		Max     int      `yaml:"max"`
	} `yaml:"variables"`
	Netbridge []struct {
		Name     string `yaml:"name"`
		Protocol string `yaml:"protocol"`
		Default  int    `yaml:"default"`
		Required bool   `yaml:"required"`
	} `yaml:"netbridge"`
	Targets map[string]Target `yaml:"targets"`
}

// Target is one entry of targets:.
type Target struct {
	Tools     []string             `yaml:"tools"`
	Exports   map[string]yaml.Node `yaml:"exports"`
	Lifecycle map[string][]Step    `yaml:"lifecycle"`
	Methods   map[string]struct {
		AvailableIn []string `yaml:"available_in"`
		Steps       []Step   `yaml:"steps"`
	} `yaml:"methods"`
}

// Step is one lifecycle or method step. Overrideable fields stay as raw nodes.
type Step struct {
	Type     string    `yaml:"type"`
	Title    string    `yaml:"title"`
	Command  yaml.Node `yaml:"command"`
	URL      yaml.Node `yaml:"url"`
	To       yaml.Node `yaml:"to"`
	From     yaml.Node `yaml:"from"`
	Checksum yaml.Node `yaml:"checksum"`
	Signal   string    `yaml:"signal"`
	Timeout  string    `yaml:"timeout"`
}

var fence = regexp.MustCompile("(?s)```(arrow|collection)\n(.*?)\n```")

// Arrows names the arrow files of the collection, relative to arrows/.
var Arrows = []string{"minecraft", "java", "mcserver"}

// ReadArrow returns the raw markdown of arrows/<name>.md.
func ReadArrow(name string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join("..", "arrows", name+".md"))
	if err != nil {
		return nil, fmt.Errorf("read arrow %s: %w", name, err)
	}
	return raw, nil
}

// LoadArrow reads and decodes the arrow block of arrows/<name>.md.
func LoadArrow(name string) (*Manifest, error) {
	raw, err := ReadArrow(name)
	if err != nil {
		return nil, err
	}
	m, _, err := Parse(raw)
	return m, err
}

// Parse decodes the first arrow block of an ARROW.md-style document.
func Parse(doc []byte) (*Manifest, []byte, error) {
	for _, found := range fence.FindAllSubmatch(doc, -1) {
		if string(found[1]) != "arrow" {
			continue
		}
		var m Manifest
		if err := yaml.Unmarshal(found[2], &m); err != nil {
			return nil, nil, fmt.Errorf("decode manifest: %w", err)
		}
		return &m, found[2], nil
	}
	return nil, nil, fmt.Errorf("no arrow block in manifest")
}

// Overrideable resolves an Overrideable scalar node for a platform the way
// Quiver does: an exact key wins over a glob, a glob over "default".
func Overrideable(n yaml.Node, platform string) (string, bool) {
	if n.Kind == yaml.ScalarNode {
		return n.Value, true
	}
	if n.Kind != yaml.MappingNode {
		return "", false
	}
	goos, _, _ := strings.Cut(platform, "/")
	exact, glob, def := "", "", ""
	var hasExact, hasGlob, hasDef bool
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1].Value
		switch key {
		case platform:
			exact, hasExact = val, true
		case goos + "/*", "*":
			glob, hasGlob = val, true
		case "default":
			def, hasDef = val, true
		}
	}
	switch {
	case hasExact:
		return exact, true
	case hasGlob:
		return glob, true
	default:
		return def, hasDef
	}
}

var ref = regexp.MustCompile(`\$\{([A-Za-z0-9_./-]+)\}`)

// Render substitutes ${NAME} references the way Quiver does: resolved names
// are replaced, anything else stays verbatim.
func Render(s string, vars map[string]string) string {
	return ref.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := vars[m[2:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

// References lists every ${NAME} token in s.
func References(s string) []string {
	var out []string
	for _, m := range ref.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}
