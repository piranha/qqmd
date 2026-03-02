// Package config manages KDL-based collection configuration for qqmd.
// Config is stored at ~/.config/qqmd/{indexName}.kdl (respects XDG_CONFIG_HOME).
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	kdl "github.com/sblinch/kdl-go"
	"github.com/sblinch/kdl-go/document"
)

type Collection struct {
	Path             string
	Pattern          string
	Context          map[string]string
	Update           string
	IncludeByDefault *bool
}

// EmbedConfig configures the embedding backend.
type EmbedConfig struct {
	Backend string // "openai", "ollama", or "" (auto-detect)
	Model   string // model name (e.g. "text-embedding-3-small", "nomic-embed-text")
	APIKey  string // API key (for openai backend)
	BaseURL string // base URL override (for openai-compatible APIs)
}

type Config struct {
	GlobalContext string
	Embed         EmbedConfig
	Collections   map[string]Collection
}

type NamedCollection struct {
	Name string
	Collection
}

var indexName = "index"

func SetIndexName(name string) {
	if strings.Contains(name, "/") {
		abs, err := filepath.Abs(name)
		if err == nil {
			name = strings.ReplaceAll(abs, "/", "_")
			name = strings.TrimPrefix(name, "_")
		}
	}
	indexName = name
}

func configDir() string {
	if d := os.Getenv("QQMD_CONFIG_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "qqmd")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "qqmd")
}

func ConfigPath() string {
	return filepath.Join(configDir(), indexName+".kdl")
}

func newNode(name string) *document.Node {
	n := document.NewNode()
	n.SetName(name)
	return n
}

// marshalConfig converts a Config to a KDL document.
func marshalConfig(cfg *Config) ([]byte, error) {
	doc := document.New()

	if cfg.GlobalContext != "" {
		n := newNode("global-context")
		n.AddArgument(cfg.GlobalContext, "")
		doc.AddNode(n)
	}

	if cfg.Embed.Backend != "" || cfg.Embed.Model != "" || cfg.Embed.APIKey != "" || cfg.Embed.BaseURL != "" {
		n := newNode("embed")
		if cfg.Embed.Backend != "" {
			backendNode := newNode("backend")
			backendNode.AddArgument(cfg.Embed.Backend, "")
			n.AddNode(backendNode)
		}
		if cfg.Embed.Model != "" {
			modelNode := newNode("model")
			modelNode.AddArgument(cfg.Embed.Model, "")
			n.AddNode(modelNode)
		}
		if cfg.Embed.APIKey != "" {
			keyNode := newNode("api-key")
			keyNode.AddArgument(cfg.Embed.APIKey, "")
			n.AddNode(keyNode)
		}
		if cfg.Embed.BaseURL != "" {
			urlNode := newNode("base-url")
			urlNode.AddArgument(cfg.Embed.BaseURL, "")
			n.AddNode(urlNode)
		}
		doc.AddNode(n)
	}

	for name, coll := range cfg.Collections {
		n := newNode("collection")
		n.AddArgument(name, "")

		pathNode := newNode("path")
		pathNode.AddArgument(coll.Path, "")
		n.AddNode(pathNode)

		patternNode := newNode("pattern")
		patternNode.AddArgument(coll.Pattern, "")
		n.AddNode(patternNode)

		if coll.Update != "" {
			updateNode := newNode("update")
			updateNode.AddArgument(coll.Update, "")
			n.AddNode(updateNode)
		}

		if coll.IncludeByDefault != nil {
			inclNode := newNode("include-by-default")
			inclNode.AddArgument(*coll.IncludeByDefault, "")
			n.AddNode(inclNode)
		}

		for path, desc := range coll.Context {
			ctxNode := newNode("context")
			ctxNode.AddArgument(path, "")
			ctxNode.AddArgument(desc, "")
			n.AddNode(ctxNode)
		}

		doc.AddNode(n)
	}

	var buf bytes.Buffer
	if err := kdl.Generate(doc, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unmarshalConfig parses a KDL document into a Config.
func unmarshalConfig(data []byte) (*Config, error) {
	doc, err := kdl.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Collections: make(map[string]Collection),
	}

	for _, node := range doc.Nodes {
		switch node.Name.String() {
		case "global-context":
			if len(node.Arguments) > 0 {
				cfg.GlobalContext = fmt.Sprintf("%v", node.Arguments[0].Value)
			}
		case "embed":
			for _, child := range node.Children {
				switch child.Name.String() {
				case "backend":
					if len(child.Arguments) > 0 {
						cfg.Embed.Backend = fmt.Sprintf("%v", child.Arguments[0].Value)
					}
				case "model":
					if len(child.Arguments) > 0 {
						cfg.Embed.Model = fmt.Sprintf("%v", child.Arguments[0].Value)
					}
				case "api-key":
					if len(child.Arguments) > 0 {
						cfg.Embed.APIKey = fmt.Sprintf("%v", child.Arguments[0].Value)
					}
				case "base-url":
					if len(child.Arguments) > 0 {
						cfg.Embed.BaseURL = fmt.Sprintf("%v", child.Arguments[0].Value)
					}
				}
			}
		case "collection":
			if len(node.Arguments) == 0 {
				continue
			}
			name := fmt.Sprintf("%v", node.Arguments[0].Value)
			coll := Collection{
				Context: make(map[string]string),
			}
			for _, child := range node.Children {
				switch child.Name.String() {
				case "path":
					if len(child.Arguments) > 0 {
						coll.Path = fmt.Sprintf("%v", child.Arguments[0].Value)
					}
				case "pattern":
					if len(child.Arguments) > 0 {
						coll.Pattern = fmt.Sprintf("%v", child.Arguments[0].Value)
					}
				case "update":
					if len(child.Arguments) > 0 {
						coll.Update = fmt.Sprintf("%v", child.Arguments[0].Value)
					}
				case "include-by-default":
					if len(child.Arguments) > 0 {
						v, ok := child.Arguments[0].Value.(bool)
						if ok {
							coll.IncludeByDefault = &v
						}
					}
				case "context":
					if len(child.Arguments) >= 2 {
						path := fmt.Sprintf("%v", child.Arguments[0].Value)
						desc := fmt.Sprintf("%v", child.Arguments[1].Value)
						coll.Context[path] = desc
					}
				}
			}
			if len(coll.Context) == 0 {
				coll.Context = nil
			}
			cfg.Collections[name] = coll
		}
	}

	return cfg, nil
}

func LoadConfig() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Collections: make(map[string]Collection)}, nil
	}
	if err != nil {
		return nil, err
	}
	cfg, err := unmarshalConfig(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Collections == nil {
		cfg.Collections = make(map[string]Collection)
	}
	return cfg, nil
}

func SaveConfig(cfg *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0o644)
}

func GetCollection(name string) (*NamedCollection, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	c, ok := cfg.Collections[name]
	if !ok {
		return nil, nil
	}
	return &NamedCollection{Name: name, Collection: c}, nil
}

func ListCollections() ([]NamedCollection, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	result := make([]NamedCollection, 0, len(cfg.Collections))
	for name, c := range cfg.Collections {
		result = append(result, NamedCollection{Name: name, Collection: c})
	}
	return result, nil
}

func GetDefaultCollectionNames() ([]string, error) {
	colls, err := ListCollections()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, c := range colls {
		if c.IncludeByDefault == nil || *c.IncludeByDefault {
			names = append(names, c.Name)
		}
	}
	return names, nil
}

func AddCollection(name, path, pattern string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if pattern == "" {
		pattern = "**/*.md"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	existing, ok := cfg.Collections[name]
	var ctx map[string]string
	if ok {
		ctx = existing.Context
	}
	cfg.Collections[name] = Collection{
		Path:    abs,
		Pattern: pattern,
		Context: ctx,
	}
	return SaveConfig(cfg)
}

func RemoveCollection(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Collections[name]; !ok {
		return fmt.Errorf("collection %q not found", name)
	}
	delete(cfg.Collections, name)
	return SaveConfig(cfg)
}

func RenameCollection(oldName, newName string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	c, ok := cfg.Collections[oldName]
	if !ok {
		return fmt.Errorf("collection %q not found", oldName)
	}
	if _, exists := cfg.Collections[newName]; exists {
		return fmt.Errorf("collection %q already exists", newName)
	}
	cfg.Collections[newName] = c
	delete(cfg.Collections, oldName)
	return SaveConfig(cfg)
}

func UpdateCollectionSettings(name string, update *string, includeByDefault *bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	c, ok := cfg.Collections[name]
	if !ok {
		return fmt.Errorf("collection %q not found", name)
	}
	if update != nil {
		c.Update = *update
	}
	if includeByDefault != nil {
		c.IncludeByDefault = includeByDefault
	}
	cfg.Collections[name] = c
	return SaveConfig(cfg)
}

// Context management

func AddContext(collectionName, pathPrefix, description string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	c, ok := cfg.Collections[collectionName]
	if !ok {
		return fmt.Errorf("collection %q not found", collectionName)
	}
	if c.Context == nil {
		c.Context = make(map[string]string)
	}
	c.Context[pathPrefix] = description
	cfg.Collections[collectionName] = c
	return SaveConfig(cfg)
}

func RemoveContext(collectionName, pathPrefix string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	c, ok := cfg.Collections[collectionName]
	if !ok {
		return fmt.Errorf("collection %q not found", collectionName)
	}
	if c.Context == nil {
		return fmt.Errorf("no contexts for collection %q", collectionName)
	}
	if _, ok := c.Context[pathPrefix]; !ok {
		return fmt.Errorf("context %q not found in collection %q", pathPrefix, collectionName)
	}
	delete(c.Context, pathPrefix)
	if len(c.Context) == 0 {
		c.Context = nil
	}
	cfg.Collections[collectionName] = c
	return SaveConfig(cfg)
}

type ContextEntry struct {
	Collection  string
	Path        string
	Description string
}

func ListAllContexts() ([]ContextEntry, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	var entries []ContextEntry
	if cfg.GlobalContext != "" {
		entries = append(entries, ContextEntry{Collection: "*", Path: "/", Description: cfg.GlobalContext})
	}
	for name, c := range cfg.Collections {
		for path, desc := range c.Context {
			entries = append(entries, ContextEntry{Collection: name, Path: path, Description: desc})
		}
	}
	return entries, nil
}

func FindContextForPath(collectionName, filePath string) string {
	cfg, err := LoadConfig()
	if err != nil {
		return ""
	}
	c, ok := cfg.Collections[collectionName]
	if !ok || c.Context == nil {
		return cfg.GlobalContext
	}
	// Find longest matching prefix
	bestLen := -1
	bestCtx := ""
	for prefix, ctx := range c.Context {
		np := prefix
		if !strings.HasPrefix(np, "/") {
			np = "/" + np
		}
		nf := filePath
		if !strings.HasPrefix(nf, "/") {
			nf = "/" + nf
		}
		if strings.HasPrefix(nf, np) && len(np) > bestLen {
			bestLen = len(np)
			bestCtx = ctx
		}
	}
	if bestCtx != "" {
		return bestCtx
	}
	return cfg.GlobalContext
}

// GetEmbedConfig returns the effective embedding configuration,
// merging the config file with environment variable overrides.
// Environment variables take precedence over the config file.
func GetEmbedConfig() EmbedConfig {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = &Config{}
	}
	ec := cfg.Embed

	// Env overrides config
	if v := os.Getenv("QQMD_EMBED_BACKEND"); v != "" {
		ec.Backend = v
	}
	if v := os.Getenv("QQMD_EMBED_MODEL"); v != "" {
		ec.Model = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		ec.APIKey = v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		ec.BaseURL = v
	}

	return ec
}

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func IsValidCollectionName(name string) bool {
	return validNameRe.MatchString(name)
}
