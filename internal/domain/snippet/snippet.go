package snippet

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	maxNameLength     = 120
	maxFolderLength   = 120
	maxTagLength      = 64
	maxTags           = 32
	maxBodyLength     = 32 * 1024
	maxValueLength    = 4 * 1024
	maxRenderedLength = 64 * 1024
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Snippet struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Folder    string    `json:"folder"`
	Tags      []string  `json:"tags"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s Snippet) WithDefaults(now time.Time) Snippet {
	s.ID = strings.TrimSpace(s.ID)
	s.Name = strings.TrimSpace(s.Name)
	s.Folder = strings.TrimSpace(s.Folder)
	s.Tags = normalizeTags(s.Tags)
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now.UTC()
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = s.CreatedAt
	}
	return s
}

func (s Snippet) Validate() error {
	if s.ID == "" {
		return errors.New("snippet id is required")
	}
	if err := validateLabel("name", s.Name, maxNameLength, false); err != nil {
		return err
	}
	if err := validateLabel("folder", s.Folder, maxFolderLength, true); err != nil {
		return err
	}
	if len(s.Tags) > maxTags {
		return fmt.Errorf("snippet has more than %d tags", maxTags)
	}
	seenTags := make(map[string]struct{}, len(s.Tags))
	for _, tag := range s.Tags {
		if err := validateLabel("tag", tag, maxTagLength, false); err != nil {
			return err
		}
		key := strings.ToLower(tag)
		if _, exists := seenTags[key]; exists {
			return fmt.Errorf("duplicate snippet tag %q", tag)
		}
		seenTags[key] = struct{}{}
	}
	if strings.TrimSpace(s.Body) == "" {
		return errors.New("snippet body is required")
	}
	if len(s.Body) > maxBodyLength {
		return fmt.Errorf("snippet body exceeds %d bytes", maxBodyLength)
	}
	if strings.IndexByte(s.Body, 0) >= 0 {
		return errors.New("snippet body contains a NUL byte")
	}
	_, err := Variables(s.Body)
	return err
}

func Variables(body string) ([]string, error) {
	placeholders, err := parsePlaceholders(body)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(placeholders))
	result := make([]string, 0, len(placeholders))
	for _, placeholder := range placeholders {
		if _, exists := seen[placeholder.name]; exists {
			continue
		}
		seen[placeholder.name] = struct{}{}
		result = append(result, placeholder.name)
	}
	return result, nil
}

func Render(body string, values map[string]string) (string, error) {
	placeholders, err := parsePlaceholders(body)
	if err != nil {
		return "", err
	}
	known := make(map[string]struct{}, len(placeholders))
	for _, placeholder := range placeholders {
		known[placeholder.name] = struct{}{}
	}
	for name, value := range values {
		if _, exists := known[name]; !exists {
			return "", fmt.Errorf("unknown snippet variable %q", name)
		}
		if len(value) > maxValueLength {
			return "", fmt.Errorf("snippet variable %q exceeds %d bytes", name, maxValueLength)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return "", fmt.Errorf("snippet variable %q contains a NUL byte", name)
		}
	}

	var rendered strings.Builder
	position := 0
	for _, placeholder := range placeholders {
		value, exists := values[placeholder.name]
		if !exists {
			return "", fmt.Errorf("snippet variable %q is required", placeholder.name)
		}
		rendered.WriteString(body[position:placeholder.start])
		rendered.WriteString(value)
		if rendered.Len() > maxRenderedLength {
			return "", fmt.Errorf("rendered snippet exceeds %d bytes", maxRenderedLength)
		}
		position = placeholder.end
	}
	rendered.WriteString(body[position:])
	if rendered.Len() > maxRenderedLength {
		return "", fmt.Errorf("rendered snippet exceeds %d bytes", maxRenderedLength)
	}
	return rendered.String(), nil
}

type placeholder struct {
	start int
	end   int
	name  string
}

func parsePlaceholders(body string) ([]placeholder, error) {
	result := make([]placeholder, 0)
	position := 0
	for position < len(body) {
		openOffset := strings.Index(body[position:], "{{")
		closeOffset := strings.Index(body[position:], "}}")
		if closeOffset >= 0 && (openOffset < 0 || closeOffset < openOffset) {
			return nil, errors.New("snippet contains an unmatched closing placeholder")
		}
		if openOffset < 0 {
			break
		}
		start := position + openOffset
		closeRelative := strings.Index(body[start+2:], "}}")
		if closeRelative < 0 {
			return nil, errors.New("snippet contains an unclosed placeholder")
		}
		end := start + 2 + closeRelative
		if strings.Contains(body[start+2:end], "{{") {
			return nil, errors.New("snippet contains a nested placeholder")
		}
		name := strings.TrimSpace(body[start+2 : end])
		if !variableNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid snippet variable %q", name)
		}
		result = append(result, placeholder{start: start, end: end + 2, name: name})
		position = end + 2
	}
	return result, nil
}

func validateLabel(field, value string, limit int, optional bool) error {
	if value == "" {
		if optional {
			return nil
		}
		return fmt.Errorf("snippet %s is required", field)
	}
	if len(value) > limit {
		return fmt.Errorf("snippet %s exceeds %d bytes", field, limit)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("snippet %s contains a control character", field)
		}
	}
	return nil
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}
