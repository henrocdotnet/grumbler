package rules

import (
	"reflect"
	"testing"

	"github.com/henrocdotnet/grumbler/internal/config"
	"gopkg.in/yaml.v3"
)

// Init and unconfigured repositories must use the same policy. Otherwise a
// rules improvement only reaches one of the two review paths.
func TestDefaultRulesMatchInitScaffold(t *testing.T) {
	var scaffold RulesFile
	if err := yaml.Unmarshal([]byte(config.DefaultRulesYAML), &scaffold); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(DefaultRules(), scaffold.Rules) {
		t.Fatal("built-in rules and init scaffold diverged")
	}
}
