package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/henrocdotnet/grumbler/internal/model"
)

// WriteJSON renders suggestions as formatted JSON.
func WriteJSON(w io.Writer, result model.ReviewResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w)
	return err
}
