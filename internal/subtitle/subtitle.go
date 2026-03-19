package subtitle

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

var markerStartRe = regexp.MustCompile(`<!-- subtitle:([^:]+):start sha256:([a-f0-9]+) -->`)
var markerEndRe = regexp.MustCompile(`<!-- subtitle:([^:]+):end -->`)

func markerStart(lang, hash string) string {
	return fmt.Sprintf("<!-- subtitle:%s:start sha256:%s -->", lang, hash)
}

func markerEnd(lang string) string {
	return fmt.Sprintf("<!-- subtitle:%s:end -->", lang)
}

// computeHash returns the first 8 hex chars of the SHA256 of s.
func computeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}

// StripTranslation removes all subtitle marker blocks from body.
func StripTranslation(body string) string {
	return stripTranslationForLang(body, "")
}

// StripTranslationForLang removes subtitle marker blocks for a specific language.
func StripTranslationForLang(body, lang string) string {
	return stripTranslationForLang(body, lang)
}

func stripTranslationForLang(body, lang string) string {
	lines := strings.Split(body, "\n")
	var result []string
	inBlock := false
	blockLang := ""

	for _, line := range lines {
		if m := markerStartRe.FindStringSubmatch(line); m != nil {
			if lang == "" || m[1] == lang {
				inBlock = true
				blockLang = m[1]
				continue
			}
		}
		if inBlock {
			if m := markerEndRe.FindStringSubmatch(line); m != nil && m[1] == blockLang {
				inBlock = false
				blockLang = ""
				continue
			}
			continue
		}
		result = append(result, line)
	}

	out := strings.Join(result, "\n")
	out = strings.TrimRight(out, "\n")
	return out
}

// NeedsTranslation returns true if the body needs translation for the given language.
func NeedsTranslation(body, lang string) bool {
	start := fmt.Sprintf("<!-- subtitle:%s:start sha256:", lang)
	_, after, found := strings.Cut(body, start)
	if !found {
		return true
	}

	existingHash, _, found := strings.Cut(after, " -->")
	if !found {
		return true
	}

	original := StripTranslation(body)
	currentHash := computeHash(original)

	return existingHash != currentHash
}

// ApplyTranslation inserts or replaces the translation block for the given language.
func ApplyTranslation(body, translation, lang, model string) (string, error) {
	original := StripTranslation(body)
	hash := computeHash(original)

	start := markerStart(lang, hash)
	end := markerEnd(lang)

	block := formatBlock(translation, model)

	existingStart := fmt.Sprintf("<!-- subtitle:%s:start sha256:", lang)
	if strings.Contains(body, existingStart) {
		return replaceExistingBlock(body, block, lang, hash), nil
	}

	return body + "\n\n" + start + "\n" + block + "\n" + end, nil
}

func formatBlock(translation, model string) string {
	return fmt.Sprintf("---\n%s\n\n---\n<sub>Translated by [gh-subtitle](https://github.com/k1LoW/gh-subtitle) (model: %s)</sub>", translation, model)
}

// replaceExistingBlock replaces the marker block for the given language using line-based processing.
func replaceExistingBlock(body, newContent, lang, hash string) string {
	lines := strings.Split(body, "\n")
	var result []string
	inBlock := false

	for _, line := range lines {
		if m := markerStartRe.FindStringSubmatch(line); m != nil && m[1] == lang {
			inBlock = true
			// Write new start marker and content
			result = append(result, markerStart(lang, hash))
			result = append(result, newContent)
			continue
		}
		if inBlock {
			if m := markerEndRe.FindStringSubmatch(line); m != nil && m[1] == lang {
				inBlock = false
				result = append(result, line)
				continue
			}
			// Skip old content
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}
