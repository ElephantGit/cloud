// Package skilldoc validates reusable Agent skill documents and their SKILL.md wire format.
package skilldoc

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	// MaxManifestBytes bounds one imported SKILL.md before parsing.
	MaxManifestBytes = 1 << 20
	// MaxContentBytes bounds the Markdown body stored for one skill.
	MaxContentBytes = 900 << 10
	// MaxDescriptionBytes matches desktop's public skill contract.
	MaxDescriptionBytes = 4096
)

// Error is a stable, user-actionable validation failure.
type Error struct {
	Code string
}

func (e *Error) Error() string { return e.Code }

func fail(code string) error { return &Error{Code: code} }

var (
	frontMatter = regexp.MustCompile(`(?m)^---\r?$`)
	namePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Document is the validated data stored for one skill.
type Document struct {
	Name        string
	Description string
	Content     string
}

// Parse reads one SKILL.md with YAML front matter containing name and description.
func Parse(raw []byte) (Document, error) {
	if len(raw) > MaxManifestBytes {
		return Document{}, fail("skill_file_too_large")
	}
	if !utf8.Valid(raw) {
		return Document{}, fail("skill_invalid_encoding")
	}
	text := strings.TrimPrefix(string(raw), "\ufeff")
	matches := frontMatter.FindAllStringIndex(text, -1)
	if len(matches) < 2 || matches[0][0] != 0 {
		return Document{}, fail("skill_front_matter_missing")
	}
	var manifest struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if e := yaml.Unmarshal([]byte(text[matches[0][1]:matches[1][0]]), &manifest); e != nil {
		return Document{}, fail("skill_front_matter_invalid")
	}
	body := strings.TrimLeft(text[matches[1][1]:], "\r\n")
	return Validate(Document{Name: manifest.Name, Description: manifest.Description, Content: body})
}

// Validate applies the same public-name, description and content rules to edits and imports.
func Validate(d Document) (Document, error) {
	d.Name = strings.TrimSpace(d.Name)
	d.Description = strings.TrimSpace(d.Description)
	if err := ValidateName(d.Name); err != nil {
		return Document{}, err
	}
	if err := ValidateDescription(d.Description); err != nil {
		return Document{}, err
	}
	if len(d.Content) > MaxContentBytes {
		return Document{}, fail("skill_content_too_large")
	}
	if !utf8.ValidString(d.Content) || strings.IndexByte(d.Content, 0) >= 0 {
		return Document{}, fail("skill_content_invalid")
	}
	return d, nil
}

// ValidateName enforces one filesystem-safe ASCII path segment that cannot start with a dot.
func ValidateName(name string) error {
	if name == "" {
		return fail("skill_name_missing")
	}
	if strings.HasPrefix(name, ".") || len(name) > 255 || len([]rune(name)) > 255 || !namePattern.MatchString(name) {
		return fail("skill_name_invalid")
	}
	return nil
}

// ValidateDescription matches the public skill description contract.
func ValidateDescription(description string) error {
	if description == "" {
		return fail("skill_description_missing")
	}
	if len(description) > MaxDescriptionBytes {
		return fail("skill_description_too_large")
	}
	if strings.IndexByte(description, 0) >= 0 {
		return fail("skill_description_invalid")
	}
	return nil
}

// Render produces the canonical SKILL.md an Agent discovers on disk.
func Render(d Document) string {
	var b strings.Builder
	b.WriteString("---\nname: ")
	b.WriteString(strconv.Quote(d.Name))
	b.WriteString("\ndescription: ")
	b.WriteString(strconv.Quote(d.Description))
	b.WriteString("\n---\n\n")
	b.WriteString(d.Content)
	if !strings.HasSuffix(d.Content, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

// Code returns the stable error code for an Error and "" for any other error.
func Code(err error) string {
	var validation *Error
	if errors.As(err, &validation) {
		return validation.Code
	}
	return ""
}
