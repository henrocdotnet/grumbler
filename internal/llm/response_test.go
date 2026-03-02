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

func TestExtractJSON_InvalidBoundaryScanCandidate(t *testing.T) {
	// Balanced braces but not valid JSON — exercises the new debug log path.
	// Boundary scan finds {not: valid json} at depth 0, json.Valid fails,
	// falls through to passthrough.
	input := `text {not: valid json} more text`
	got := ExtractJSON(input)
	if got != input {
		t.Errorf("got %q, want passthrough %q", got, input)
	}
}

func TestExtractJSON_ProseOnly(t *testing.T) {
	input := `This is plain prose with no JSON structure at all.`
	got := ExtractJSON(input)
	if got != input {
		t.Errorf("got %q, want passthrough %q", got, input)
	}
}

func TestExtractJSON_EscapedQuotes(t *testing.T) {
	// Embedded JSON with escaped quotes — exercises inString/backslash-skip in boundary scan.
	input := `prefix {"key":"val\"ue"} suffix`
	want := `{"key":"val\"ue"}`
	got := ExtractJSON(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
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
