package prompt

import (
	"os"
	"path/filepath"
	"text/template"

	"github.com/henrocdotnet/grumbler/internal/config"
)

var overrideTmpls *template.Template

// LoadOverrides reads prompt overrides from the config directory.
func LoadOverrides(dir string) error {
	promptDir := filepath.Join(dir, config.Dir, "prompts")
	entries, err := os.ReadDir(promptDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no overrides
		}
		return err
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".tmpl" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(promptDir, e.Name()))
		if err != nil {
			return err
		}
		if overrideTmpls == nil {
			overrideTmpls, err = template.New(e.Name()).Parse(string(data))
		} else {
			_, err = overrideTmpls.New(e.Name()).Parse(string(data))
		}
		if err != nil {
			return err
		}
	}
	return nil
}
