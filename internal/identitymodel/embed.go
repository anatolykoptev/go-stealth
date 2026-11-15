package identitymodel

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed model.json
var modelJSON []byte

var (
	loadOnce sync.Once
	loaded   *File
	loadErr  error
)

// Default returns the model shipped with the library, parsed and validated
// once.
func Default() (*File, error) {
	loadOnce.Do(func() { loaded, loadErr = Parse(modelJSON) })
	return loaded, loadErr
}

// Parse reads and validates a model file.
func Parse(b []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("identitymodel: %w", err)
	}
	if f.Format != FormatV1 {
		return nil, fmt.Errorf("identitymodel: unsupported format %q", f.Format)
	}
	for name, nw := range f.Networks {
		if err := nw.Index(); err != nil {
			return nil, fmt.Errorf("network %q: %w", name, err)
		}
	}
	return &f, nil
}
