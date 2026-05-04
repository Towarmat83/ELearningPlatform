package content

// Course is the in-memory representation of a course loaded from disk.
type Course struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Difficulty  string   `json:"difficulty"`
	IsPublished bool     `json:"is_published"`
	Lessons     []Lesson `json:"lessons"`
	// Source is "local" or the git repo URL (when synced from a git repo).
	Source string `json:"source,omitempty"`
}

// Lesson is a single lesson inside a Course.
type Lesson struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Order   int    `json:"order"`
	Content string `json:"content"` // raw Markdown
}
