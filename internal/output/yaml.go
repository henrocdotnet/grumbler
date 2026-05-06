package output

import (
	"fmt"
	"io"

	"github.com/henrocdotnet/grumbler/internal/model"
	"gopkg.in/yaml.v3"
)

// WriteYAML renders suggestions as formatted YAML.
func WriteYAML(w io.Writer, result model.ReviewResult) error {
	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshaling YAML: %w", err)
	}
	_, err = w.Write(data)
	return err
}
