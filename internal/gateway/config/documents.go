package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

func readDocuments(paths []string) ([]document, error) {
	var docs []document
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		files := []string{path}
		if info.IsDir() {
			files, err = filepath.Glob(filepath.Join(path, "*.y*ml"))
			if err != nil {
				return nil, err
			}
		}
		for _, file := range files {
			batch, err := readFile(file)
			if err != nil {
				return nil, err
			}
			docs = append(docs, batch...)
		}
	}
	if len(docs) == 0 {
		return nil, errors.New("no configuration documents")
	}
	sort.SliceStable(docs, func(i, j int) bool {
		if docs[i].secretPriority != docs[j].secretPriority {
			return docs[i].secretPriority < docs[j].secretPriority
		}
		return docs[i].weight < docs[j].weight
	})
	return docs, nil
}
func readFile(file string) ([]document, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	decoder := yaml.NewDecoder(f)
	var docs []document
	for {
		var data map[string]any
		err = decoder.Decode(&data)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: malformed YAML", file)
		}
		if len(data) == 0 {
			continue
		}
		if err = noNull(data); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		d := document{data: data, file: file}
		if err = d.parseWeight(); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
}
func (d *document) parseWeight() error {
	w, ok := d.data["weight"]
	if !ok {
		return nil
	}
	switch v := w.(type) {
	case int:
		d.weight = v
	case string:
		if d.data["kind"] != "gateway/secret/v1" || (v != "min" && v != "max") {
			return errors.New("invalid weight")
		}
		if v == "max" {
			d.secretPriority = 1
		} else {
			d.secretPriority = -1
		}
	default:
		return errors.New("invalid weight")
	}
	return nil
}
func noNull(v any) error {
	if v == nil {
		return errors.New("null is unsupported")
	}
	switch x := v.(type) {
	case map[string]any:
		for _, v := range x {
			if err := noNull(v); err != nil {
				return err
			}
		}
	case []any:
		for _, v := range x {
			if err := noNull(v); err != nil {
				return err
			}
		}
	}
	return nil
}
func merge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			dm, ok := dst[k].(map[string]any)
			if !ok {
				dm = map[string]any{}
				dst[k] = dm
			}
			merge(dm, sm)
		} else {
			dst[k] = v
		}
	}
}
