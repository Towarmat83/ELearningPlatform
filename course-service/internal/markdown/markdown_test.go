package markdown

import (
	"reflect"
	"strings"
	"testing"

	"github.com/genesary/pupitre/course-service/internal/content"
	"github.com/genesary/pupitre/course-service/internal/definition"
)

// TestImportSplitsOnHeadings checks frontmatter, heading splitting, fenced
// code and module directives on one document.
func TestImportSplitsOnHeadings(t *testing.T) {
	t.Parallel()

	doc := `---
slug: linux-intro
title: Linux Intro
category: linux
public: true
---

# Linux Intro

Bienvenue.

## What is Linux

A kernel.

` + "```sh\n## not a heading\n```" + `

## Quiz

<!--pupitre
type: quiz
passingScore: 80
-->
`

	res, err := Import([]byte(doc), Options{Split: SplitH2})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if res.Slug != "linux-intro" || res.Spec.Title != "Linux Intro" || !res.Spec.Public {
		t.Fatalf("frontmatter not applied: %+v", res.Spec)
	}

	if len(res.Spec.Modules) != 3 {
		t.Fatalf("want 3 modules, got %d: %+v", len(res.Spec.Modules), res.Spec.Modules)
	}

	if got := res.Spec.Modules[0]; got.Name != "Linux Intro" || got.InlineContent != "Bienvenue." {
		t.Errorf("preamble module = %+v", got)
	}

	if got := res.Spec.Modules[1]; got.Name != "What is Linux" || !strings.Contains(got.InlineContent, "## not a heading") {
		t.Errorf("fenced heading split the module: %+v", got)
	}

	if got := res.Spec.Modules[2]; got.Type != "quiz" || got.PassingScore != 80 {
		t.Errorf("directive not applied: %+v", got)
	}
}

// TestImportSingleModuleUsesLeadingHeading checks that an unsplit document
// takes its module name from its own title and keeps the rest as content.
func TestImportSingleModuleUsesLeadingHeading(t *testing.T) {
	t.Parallel()

	res, err := Import([]byte("# Ma leçon\n\nDu texte.\n\n## Une sous-partie\n\nEncore.\n"), Options{Slug: "lecon"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(res.Spec.Modules) != 1 {
		t.Fatalf("want 1 module, got %d", len(res.Spec.Modules))
	}

	module := res.Spec.Modules[0]
	if module.Name != "Ma leçon" {
		t.Errorf("name = %q", module.Name)
	}

	if !strings.HasPrefix(module.InlineContent, "Du texte.") || !strings.Contains(module.InlineContent, "## Une sous-partie") {
		t.Errorf("content = %q", module.InlineContent)
	}
}

// TestImportUsesFrontmatterSplit checks that a document with no requested
// split falls back to the level recorded in its frontmatter.
func TestImportUsesFrontmatterSplit(t *testing.T) {
	t.Parallel()

	res, err := Import([]byte("---\nslug: c\nsplit: h1\n---\n\n# A\n\na\n\n# B\n\nb\n"), Options{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(res.Spec.Modules) != 2 {
		t.Fatalf("want 2 modules, got %d", len(res.Spec.Modules))
	}
}

// TestImportSlugFallsBackToTitle checks the slug derived from a document
// that declares only a title.
func TestImportSlugFallsBackToTitle(t *testing.T) {
	t.Parallel()

	res, err := Import([]byte("---\ntitle: Kubernetes Basics\n---\n\ncontent\n"), Options{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if res.Slug != "kubernetes-basics" {
		t.Errorf("slug = %q", res.Slug)
	}
}

// TestCourseSlugFoldsAccents checks that a derived slug stays inside the
// [a-z0-9-] the catalog URLs and the admin forms accept.
func TestCourseSlugFoldsAccents(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"Introduction à Linux": "introduction-a-linux",
		"  Déjà  vu ! ":        "deja-vu",
		"Sécurité & Réseaux":   "securite-reseaux",
		"\u65e5\u672c\u8a9e":   "", // a title with nothing to fold yields no slug
	}

	for title, want := range cases {
		t.Run(title, func(t *testing.T) {
			t.Parallel()

			if got := courseSlug(title); got != want {
				t.Errorf("courseSlug(%q) = %q, want %q", title, got, want)
			}
		})
	}
}

// TestImportRejectsUnusableDocuments checks the documents Import refuses.
func TestImportRejectsUnusableDocuments(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"no slug":                  "just text\n",
		"unterminated header":      "---\ntitle: x\n\nbody\n",
		"unterminated directive":   "---\nslug: c\n---\n\n# A\n\n<!--pupitre\ntype: quiz\n",
		"bad split in frontmatter": "---\nslug: c\nsplit: h9\n---\n\n# A\n",
	}

	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Import([]byte(doc), Options{})
			if err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}

// TestImportWarnsWhenBodyIsIgnored checks that a module reading its content
// from git drops the markdown under its heading and says so.
func TestImportWarnsWhenBodyIsIgnored(t *testing.T) {
	t.Parallel()

	doc := "---\nslug: c\n---\n\n# Git module\n\n<!--pupitre\nsrc: https://github.com/org/repo\nref: main\npath: a.md\n-->\n\nignored body\n"

	res, err := Import([]byte(doc), Options{Split: SplitH1})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if res.Spec.Modules[0].InlineContent != "" {
		t.Error("git-backed module kept the markdown body")
	}

	if len(res.Warnings) == 0 {
		t.Error("want a warning about the dropped body")
	}
}

// TestExportRoundTrips checks that a course survives Export followed by
// Import field for field.
func TestExportRoundTrips(t *testing.T) {
	t.Parallel()

	spec := definition.Course{
		Title:       "Kubernetes Basics",
		Description: "Les fondamentaux",
		Category:    "kubernetes",
		Difficulty:  "beginner",
		Public:      true,
		Modules: []content.Module{
			{Name: "Intro", Type: "text", InlineContent: "## Pods\n\nUne *unité*.\n\n```yaml\n# heading in code\nkind: Pod\n```"},
			{Name: "Quiz", Type: "quiz", PassingScore: 70, Hidden: true, Skills: []string{"k8s"}},
			{Name: "From git", Type: "text", Src: "https://github.com/org/repo", Ref: "main", Path: "a.md"},
		},
	}

	doc, err := Export("kubernetes-basics", spec, ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	res, err := Import(doc, Options{})
	if err != nil {
		t.Fatalf("Import(Export(…)): %v\n%s", err, doc)
	}

	if res.Slug != "kubernetes-basics" {
		t.Errorf("slug = %q", res.Slug)
	}

	if !reflect.DeepEqual(res.Spec, spec) {
		t.Errorf("round trip changed the spec:\n got %+v\nwant %+v\n---\n%s", res.Spec, spec, doc)
	}
}

// TestExportOmitsDirectiveForPlainText checks that a plain text module
// exports as markdown with no comment above it.
func TestExportOmitsDirectiveForPlainText(t *testing.T) {
	t.Parallel()

	spec := definition.Course{
		Title:   "T",
		Modules: []content.Module{{Name: "Only", Type: content.ModuleTypeText, InlineContent: "hello"}},
	}

	doc, err := Export("t", spec, ExportOptions{Split: SplitH2})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if strings.Contains(string(doc), commentOpen) {
		t.Errorf("plain text module got a directive:\n%s", doc)
	}

	if !strings.Contains(string(doc), "## Only") {
		t.Errorf("requested split level ignored:\n%s", doc)
	}
}

// TestExportAvoidsHeadingLevelsUsedByBodies checks that the exporter picks
// a heading depth no module body already uses.
func TestExportAvoidsHeadingLevelsUsedByBodies(t *testing.T) {
	t.Parallel()

	spec := definition.Course{
		Modules: []content.Module{{Name: "A", Type: content.ModuleTypeText, InlineContent: "# Big\n\ntext"}},
	}

	doc, err := Export("a", spec, ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if !strings.Contains(string(doc), "\n## A\n") {
		t.Errorf("want the module heading at h2 to dodge the body's h1:\n%s", doc)
	}
}
