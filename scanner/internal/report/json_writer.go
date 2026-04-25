package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/klexas/ProbeTools/scanner/internal/model"
)

type JSONWriter struct{}

func NewJSONWriter() *JSONWriter {
	return &JSONWriter{}
}

func (w *JSONWriter) Write(path string, report model.Report) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create report directory: %w", err)
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}

	return nil
}
