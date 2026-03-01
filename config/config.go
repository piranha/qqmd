// Package config manages YAML-based collection configuration for qqmd.
// Config is stored at ~/.config/qmd/{indexName}.yml (respects XDG_CONFIG_HOME).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Collection struct {
	Path             string            `yaml:"path"`
	Pattern          string            `yaml:"pattern"`
	Context          map[string]string `yaml:"context,omitempty"`
	Update           string            `yaml:"update,omitempty"`
	IncludeByDefault *bool             `yaml:"includeByDefault,omitempty"`
}

type Config struct {
	GlobalContext string                `yaml:"global_context,omitempty"`
	Collections   map[string]Collection `yaml:"collections"`
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
	if d := os.Getenv("QMD_CONFIG_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "qmd")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "qmd")
}

func ConfigPath() string {
	return filepath.Join(configDir(), indexName+".yml")
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
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Collections == nil {
		cfg.Collections = make(map[string]Collection)
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
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

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func IsValidCollectionName(name string) bool {
	return validNameRe.MatchString(name)
}
