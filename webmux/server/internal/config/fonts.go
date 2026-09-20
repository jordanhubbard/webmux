package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const DefaultFontFamily = `ui-monospace, "SFMono-Regular", Monaco, Menlo, Consolas, "Liberation Mono", "DejaVu Sans Mono", monospace`
const FontFamilyError = "Invalid app.default_term.font_family"
const FontFacesError = "Invalid app.font_faces"

var fontScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
var fontWeight = regexp.MustCompile(`^(normal|bold|[1-9]00)$`)

func familyPart(part string) (string, error) {
	if len(part) >= 2 && (part[0] == '\'' || part[0] == '"') && part[0] == part[len(part)-1] {
		return part, nil
	}
	if strings.ContainsAny(part, `"'\`) {
		return "", invalid(FontFamilyError)
	}
	if strings.IndexFunc(part, whitespace) >= 0 {
		return `"` + part + `"`, nil
	}
	return part, nil
}

func FontFamily(value any) (string, error) {
	family := text(value, DefaultFontFamily)
	if length(family) > 256 || strings.ContainsAny(family, "\x00\r\n;{}") {
		return "", invalid(FontFamilyError)
	}
	parts := []string{}
	start := 0
	var quote byte
	for i := 0; i < len(family); i++ {
		char := family[i]
		if quote != 0 {
			if char == '\\' && i+1 < len(family) {
				i++
			} else if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
		}
		if char == ',' {
			parts = append(parts, trim(family[start:i]))
			start = i + 1
		}
	}
	if quote != 0 {
		return "", invalid(FontFamilyError)
	}
	parts = append(parts, trim(family[start:]))
	for i, part := range parts {
		if part == "" {
			return "", invalid(FontFamilyError)
		}
		normalized, err := familyPart(part)
		if err != nil {
			return "", err
		}
		parts[i] = normalized
	}
	result := strings.Join(parts, ", ")
	if length(result) > 256 {
		return "", invalid(FontFamilyError)
	}
	return result, nil
}

type FontFace struct {
	Family  string `yaml:"family" json:"family"`
	Source  string `yaml:"source" json:"source"`
	Weight  string `yaml:"weight,omitempty" json:"weight,omitempty"`
	Style   string `yaml:"style,omitempty" json:"style,omitempty"`
	Display string `yaml:"display,omitempty" json:"display,omitempty"`
	URL     string `yaml:"-" json:"url,omitempty"`
}

func FontContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".otf":
		return "font/otf"
	case ".ttf":
		return "font/ttf"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	default:
		return ""
	}
}

func FontFaces(value any) ([]FontFace, error) {
	if value == nil {
		return []FontFace{}, nil
	}
	if faces, ok := value.([]FontFace); ok {
		return slices.Clone(faces), nil
	}
	values, ok := value.([]any)
	if !ok || len(values) > 32 {
		return nil, invalid(FontFacesError)
	}
	faces := make([]FontFace, 0, len(values))
	for _, value := range values {
		raw := AsObject(value)
		family := text(raw["family"], "")
		if family == "" || length(family) > 128 || strings.ContainsAny(family, "\x00\r\n;{}\\") {
			return nil, invalid(FontFacesError)
		}
		family, err := familyPart(family)
		if err != nil {
			return nil, err
		}
		if len(family) >= 2 && (family[0] == '\'' || family[0] == '"') && family[0] == family[len(family)-1] {
			family = family[1 : len(family)-1]
		}
		source := text(raw["source"], "")
		segments := strings.FieldsFunc(source, func(r rune) bool { return r == '/' || r == '\\' })
		if source == "" || length(source) > 512 || filepath.IsAbs(source) || slices.Contains(segments, "..") || strings.ContainsAny(source, "\x00\r\n") || fontScheme.MatchString(source) || FontContentType(source) == "" {
			return nil, invalid(FontFacesError)
		}
		face := FontFace{Family: family, Source: source}
		if value := raw["weight"]; value != nil && value != "" {
			weight := text(value, "")
			switch value := value.(type) {
			case int:
				weight = fmt.Sprint(value)
			case float64:
				weight = fmt.Sprint(value)
			}
			if !fontWeight.MatchString(weight) {
				return nil, invalid(FontFacesError)
			}
			face.Weight = weight
		}
		for _, field := range []string{"style", "display"} {
			value := raw[field]
			if value == nil || value == "" {
				continue
			}
			normalized := strings.ToLower(text(value, ""))
			allowed := []string{"normal", "italic", "oblique"}
			if field == "display" {
				allowed = []string{"auto", "block", "swap", "fallback", "optional"}
			}
			if !slices.Contains(allowed, normalized) {
				return nil, invalid(FontFacesError)
			}
			if field == "style" {
				face.Style = normalized
			} else {
				face.Display = normalized
			}
		}
		faces = append(faces, face)
	}
	return faces, nil
}

func WithFontURLs(document Document) Document {
	app := clone(document.App)
	if faces, ok := app["font_faces"].([]FontFace); ok {
		faces = slices.Clone(faces)
		for i := range faces {
			faces[i].URL = fmt.Sprintf("/api/config/fonts/%d", i)
		}
		app["font_faces"] = faces
	}
	return Document{App: app}
}
