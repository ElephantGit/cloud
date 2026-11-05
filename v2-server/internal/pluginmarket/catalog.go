package pluginmarket

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Entry is one validated listing ready for the PostgreSQL catalog snapshot.
type Entry struct {
	Namespace          string
	Identifier         string
	Title              string
	Kind               string
	Version            string
	Description        string
	Homepage           string
	License            string
	Logo               any
	URL                string
	SHA256             string
	Targets            []ReleaseTarget
	PackMembers        []string
	Readme             string
	MarketplaceVisible bool
	SourceURL          string
}

// DemoHookEntry returns the package-less Hook listing used by the v2 plugin demo. The entry
// deliberately has no URL or digest: installing it only records a Space selection, and no Hook
// package or lifecycle command is ever downloaded or run.
func DemoHookEntry() Entry {
	return Entry{
		Namespace:          "demo",
		Identifier:         "sample-hooks",
		Title:              "示例 Hooks",
		Kind:               "hook",
		Version:            "1.0.0",
		Description:        "生命周期 Hook 的安装和卸载只记录空间选择，不下载或执行脚本。",
		License:            "MIT",
		Readme:             "# 示例 Hooks\n\n这是 v2 插件页的示例条目。安装和卸载只写入空间插件选择，不会下载、注册或执行任何 Hook。",
		MarketplaceVisible: true,
		SourceURL:          "本地示例",
	}
}

// WithDemoHook ensures the catalog has one Hook entry for the v2 demo. A real Hook listing from
// the synced marketplace takes precedence, so this never hides an actual package.
func WithDemoHook(entries []Entry) []Entry {
	for _, entry := range entries {
		if entry.Kind == "hook" {
			return entries
		}
	}
	return append(entries, DemoHookEntry())
}

type logoCandidate struct {
	Role      string `json:"role"`
	Extension string `json:"extension"`
	DataURL   string `json:"data,omitempty"`
}

var logoExtensions = []string{"svg", "png", "webp", "jpg", "jpeg"}

const maxLogoBytes = 256 << 10

// Scan reads every registry entry below root. Broken listings are skipped and reported while a
// directory walk failure aborts the sync so the caller keeps the previous complete snapshot.
func Scan(root, namespace, sourceURL string) ([]Entry, []string, error) {
	manifests := []string{}
	if e := filepath.WalkDir(root, func(path string, entry fs.DirEntry, e error) error {
		if e != nil || entry.IsDir() {
			return e
		}
		if entry.Name() == "orax.toml" {
			manifests = append(manifests, path)
		}
		return nil
	}); e != nil {
		return nil, nil, fmt.Errorf("walk registry: %w", e)
	}
	sort.Strings(manifests)
	entries := []Entry{}
	skipped := []string{}
	seen := map[string]bool{}
	for _, path := range manifests {
		info, e := os.Lstat(path)
		if e != nil {
			skipped = append(skipped, path+": "+e.Error())
			continue
		}
		if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
			skipped = append(skipped, path+": manifest is not a bounded regular file")
			continue
		}
		source, e := os.ReadFile(path)
		if e != nil {
			skipped = append(skipped, path+": "+e.Error())
			continue
		}
		manifest, e := parseManifest(source)
		if e != nil {
			skipped = append(skipped, path+": "+e.Error())
			continue
		}
		if !manifest.MarketplaceVisible {
			continue
		}
		id := namespace + "/" + manifest.Identifier
		if seen[id] {
			skipped = append(skipped, path+": duplicate identifier "+id)
			continue
		}
		seen[id] = true
		dir := filepath.Dir(path)
		entries = append(entries, Entry{
			Namespace: namespace, Identifier: manifest.Identifier, Title: manifest.Title,
			Kind: manifest.Kind, Version: manifest.Version, Description: manifest.Description,
			Homepage: manifest.Homepage, License: manifest.License, Logo: resolveLogo(dir),
			URL: manifest.URL, SHA256: manifest.SHA256, Targets: manifest.Targets,
			PackMembers: manifest.PackMembers, Readme: readReadme(dir),
			MarketplaceVisible: manifest.MarketplaceVisible, SourceURL: sourceURL,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Namespace+"/"+entries[i].Identifier < entries[j].Namespace+"/"+entries[j].Identifier
	})
	return entries, skipped, nil
}

func resolveLogo(dir string) any {
	best := func(prefix, role string) *logoCandidate {
		for _, extension := range logoExtensions {
			path := filepath.Join(dir, prefix+extension)
			info, e := os.Lstat(path)
			if e != nil || !info.Mode().IsRegular() || info.Size() > maxLogoBytes {
				continue
			}
			content, e := os.ReadFile(path)
			if e != nil || len(content) == 0 || len(content) > maxLogoBytes {
				continue
			}
			if extension == "svg" && !utf8.Valid(content) {
				continue
			}
			return &logoCandidate{
				Role: role, Extension: extension,
				DataURL: "data:image/" + logoMediaType(extension) + ";base64," +
					base64.StdEncoding.EncodeToString(content),
			}
		}
		return nil
	}
	light := best("logo.light.", "light")
	dark := best("logo.dark.", "dark")
	universal := best("logo.", "universal")
	candidate := func(value *logoCandidate) map[string]any {
		result := map[string]any{
			"role": value.Role, "extension": value.Extension,
		}
		if value.DataURL != "" {
			result["data"] = value.DataURL
		}
		return result
	}
	switch {
	case light != nil && dark != nil:
		return map[string]any{"light": candidate(light), "dark": candidate(dark)}
	case light != nil && universal != nil:
		return map[string]any{"light": candidate(light), "dark": candidate(universal)}
	case dark != nil && universal != nil:
		return map[string]any{"light": candidate(universal), "dark": candidate(dark)}
	case light != nil:
		return map[string]any{"universal": candidate(light)}
	case dark != nil:
		return map[string]any{"universal": candidate(dark)}
	case universal != nil:
		return map[string]any{"universal": candidate(universal)}
	default:
		return nil
	}
}

func logoMediaType(extension string) string {
	switch extension {
	case "svg":
		return "svg+xml"
	case "jpg", "jpeg":
		return "jpeg"
	default:
		return extension
	}
}

func readReadme(dir string) string {
	path := filepath.Join(dir, "README.md")
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		return ""
	}
	file, e := os.Open(path)
	if e != nil {
		return ""
	}
	defer file.Close()
	content, e := io.ReadAll(io.LimitReader(file, maxReadmeBytes))
	if e != nil || !utf8.Valid(content) {
		return ""
	}
	return string(content)
}

// NormalizeSourceURL accepts only credential-free HTTPS marketplace URLs without query or fragment.
// Tests construct Syncer directly when they need a local file URL.
func NormalizeSourceURL(value string) (string, error) {
	parsed, e := url.Parse(value)
	if e != nil {
		return "", fmt.Errorf("marketplace url: %w", e)
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("marketplace url must be credential-free HTTPS without query or fragment")
	}
	return strings.TrimSuffix(value, "/"), nil
}
