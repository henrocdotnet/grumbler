package model

// FileChange represents a single changed file in the diff.
type FileChange struct {
	Path         string     `json:"path"`
	Language     string     `json:"language"`
	Status       FileStatus `json:"status"` // added, modified, deleted, renamed
	Diff         string     `json:"diff"`   // unified diff hunks
	Content      string     `json:"-"`      // full file content (when available)
	Patch        string     `json:"-"`      // annotated patch with line numbers
	OldPath      string     `json:"oldPath,omitempty"`
	AddedLines   int        `json:"addedLines"`
	DeletedLines int        `json:"deletedLines"`
}

type FileStatus string

const (
	FileAdded    FileStatus = "added"
	FileModified FileStatus = "modified"
	FileDeleted  FileStatus = "deleted"
	FileRenamed  FileStatus = "renamed"
)

// LanguageFromPath infers language from file extension.
func LanguageFromPath(path string) string {
	exts := map[string]string{
		".go":      "go",
		".ts":      "typescript",
		".tsx":     "typescript",
		".js":      "javascript",
		".jsx":     "javascript",
		".py":      "python",
		".rb":      "ruby",
		".rs":      "rust",
		".java":    "java",
		".kt":      "kotlin",
		".swift":   "swift",
		".c":       "c",
		".cpp":     "cpp",
		".h":       "c",
		".hpp":     "cpp",
		".cs":      "csharp",
		".php":     "php",
		".scala":   "scala",
		".sh":      "bash",
		".bash":    "bash",
		".zsh":     "zsh",
		".yaml":    "yaml",
		".yml":     "yaml",
		".json":    "json",
		".md":      "markdown",
		".sql":     "sql",
		".html":    "html",
		".css":     "css",
		".scss":    "scss",
		".vue":     "vue",
		".svelte":  "svelte",
		".dart":    "dart",
		".lua":     "lua",
		".r":       "r",
		".R":       "r",
		".ex":      "elixir",
		".exs":     "elixir",
		".erl":     "erlang",
		".hs":      "haskell",
		".clj":     "clojure",
		".ml":      "ocaml",
		".tf":      "terraform",
		".proto":   "protobuf",
		".graphql": "graphql",
		".toml":    "toml",
		".ini":     "ini",
		".xml":     "xml",
	}
	for ext, lang := range exts {
		if len(path) > len(ext) && path[len(path)-len(ext):] == ext {
			return lang
		}
	}
	return "text"
}
