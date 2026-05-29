package connector

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func LoadAll(dir string) ([]Connector, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var out []Connector
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		c, err := load(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		out = append(out, c)
	}
	return out, nil
}

func load(path string) (Connector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Connector{}, err
	}
	var c Connector
	return c, yaml.Unmarshal(data, &c)
}
