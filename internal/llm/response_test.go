package llm

import "testing"

func TestExtractJSON_RawJSON(t *testing.T) {
	input := `{"suggestions": []}`
	got := ExtractJSON(input)
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestExtractJSON_Fenced(t *testing.T) {
	input := "Some preamble\n```json\n{\"key\": \"value\"}\n```\ntrailer"
	want := `{"key": "value"}`
	got := ExtractJSON(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractJSON_EmbeddedObject(t *testing.T) {
	input := `Here is the result: {"rules": [{"uuid": "abc"}]} and some more text`
	want := `{"rules": [{"uuid": "abc"}]}`
	got := ExtractJSON(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractJSON_Array(t *testing.T) {
	input := `[{"a":1},{"b":2}]`
	got := ExtractJSON(input)
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestStripCodeFences(t *testing.T) {
	input := "```go\nfunc main() {}\n```"
	want := "func main() {}"
	got := StripCodeFences(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
