package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// AnnotatePatch takes a unified diff and produces a patch with line numbers.
// Format: "L{new_line}  {content}" for context/added lines.
func AnnotatePatch(diff string) string {
	var b strings.Builder
	var newLine int

	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			// Parse hunk header: @@ -old,count +new,count @@
			newLine = parseHunkNewStart(line)
			b.WriteString(line)
			b.WriteByte('\n')
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			fmt.Fprintf(&b, "L%-5d %s\n", newLine, line)
			newLine++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			fmt.Fprintf(&b, "      %s\n", line)
			// deleted lines don't advance new line counter
		case strings.HasPrefix(line, " "):
			fmt.Fprintf(&b, "L%-5d %s\n", newLine, line)
			newLine++
		default:
			// diff headers, etc.
			if line != "" {
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func parseHunkNewStart(hunkLine string) int {
	// @@ -X,Y +Z,W @@
	idx := strings.Index(hunkLine, "+")
	if idx == -1 {
		return 1
	}
	rest := hunkLine[idx+1:]
	comma := strings.IndexByte(rest, ',')
	space := strings.IndexByte(rest, ' ')
	end := len(rest)
	if comma > 0 && comma < end {
		end = comma
	}
	if space > 0 && space < end {
		end = space
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 1
	}
	return n
}

func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
