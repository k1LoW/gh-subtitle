package subtitle

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var markerStartRe = regexp.MustCompile(`<!-- subtitle:([^:]+):start sha256:([a-f0-9]+) -->`)
var markerEndRe = regexp.MustCompile(`<!-- subtitle:([^:]+):end -->`)

var titleOriginalRe = regexp.MustCompile(`<!-- subtitle-title-original:([A-Za-z0-9+/=]+) -->`)
var titleHashRe = regexp.MustCompile(`<!-- subtitle-title:([^ ]+) sha256:([a-f0-9]+)( skip)? -->`)

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

// StripTranslation removes all subtitle marker blocks (including title markers) from body.
func StripTranslation(body string) string {
	body = StripTitleMarkers(body)
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

// ApplySkipMarker inserts or updates an empty marker block to record the content hash.
// This prevents unnecessary LLM calls on subsequent runs when the content is already in the target language.
func ApplySkipMarker(body, lang string) string {
	original := StripTranslation(body)
	hash := computeHash(original)

	start := markerStart(lang, hash)
	end := markerEnd(lang)

	existingStart := fmt.Sprintf("<!-- subtitle:%s:start sha256:", lang)
	if strings.Contains(body, existingStart) {
		return replaceExistingBlock(body, "", lang, hash)
	}

	return body + "\n\n" + start + "\n" + end
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
	return fmt.Sprintf("---\n<sub>🌐 Translated by [gh-subtitle](https://github.com/k1LoW/gh-subtitle) (model: %s)</sub>\n\n%s", model, translation)
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

// --- Title translation markers ---

const titleSeparator = " / "

// GitHubMaxTitleLength is the maximum length of a GitHub title.
const GitHubMaxTitleLength = 256

func titleOriginalMarker(title string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(title))
	return fmt.Sprintf("<!-- subtitle-title-original:%s -->", encoded)
}

func parseTitleOriginal(body string) string {
	m := titleOriginalRe.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		return ""
	}
	return string(decoded)
}

func titleHashMarker(lang, hash string) string {
	return fmt.Sprintf("<!-- subtitle-title:%s sha256:%s -->", lang, hash)
}

func titleSkipHashMarker(lang, hash string) string {
	return fmt.Sprintf("<!-- subtitle-title:%s sha256:%s skip -->", lang, hash)
}

// hasTitleMarkers returns true if the body contains any title-related markers.
func hasTitleMarkers(body string) bool {
	return titleOriginalRe.MatchString(body) || titleHashRe.MatchString(body)
}

// NeedsTitleTranslation returns true if the title needs translation for the given language.
// It compares the hash stored in the body marker against the hash of the current original title.
// If the title has been modified externally (marker's original != current title's first segment),
// re-translation is needed.
func NeedsTitleTranslation(body, title, lang string) bool {
	if title == "" {
		return false
	}

	storedOriginal := parseTitleOriginal(body)

	// Determine the current original title (what should be translated)
	currentOriginal := title
	if before, _, found := strings.Cut(title, titleSeparator); found && hasTitleMarkers(body) {
		// Only split on separator when markers exist (title is tool-managed).
		// Without markers, the title may legitimately contain " / ".
		if storedOriginal != "" && storedOriginal != before {
			return true
		}
		if storedOriginal != "" {
			currentOriginal = storedOriginal
		} else {
			currentOriginal = before
		}
	} else if storedOriginal != "" && storedOriginal != title {
		// No separator (or no markers), but stored original differs — external change.
		return true
	}

	hash := computeHash(currentOriginal)
	for _, m := range titleHashRe.FindAllStringSubmatch(body, -1) {
		if m[1] == lang {
			return m[2] != hash
		}
	}
	return true
}

// ExtractOriginalTitle extracts the original title from body markers, or returns the current title.
// Only splits on separator when title markers exist in the body (tool-managed title).
func ExtractOriginalTitle(body, title string) string {
	if orig := parseTitleOriginal(body); orig != "" {
		return orig
	}
	// Only split on separator when markers confirm the title is tool-managed.
	// Without markers, the title may legitimately contain " / ".
	if hasTitleMarkers(body) {
		if before, _, found := strings.Cut(title, titleSeparator); found {
			return before
		}
	}
	return title
}

// ApplyTitleTranslation adds or updates the title translation marker in the body for the given language.
func ApplyTitleTranslation(body, lang, originalTitle string) string {
	body = ensureTitleOriginalMarker(body, originalTitle)
	return upsertTitleHashMarker(body, lang, computeHash(originalTitle))
}

// ApplyTitleSkipMarker adds a title skip marker for same-language skip (no translation needed).
func ApplyTitleSkipMarker(body, title, lang string) string {
	originalTitle := ExtractOriginalTitle(body, title)
	body = ensureTitleOriginalMarker(body, originalTitle)
	return upsertTitleSkipHashMarker(body, lang, computeHash(originalTitle))
}

// upsertTitleHashMarker replaces or appends a title hash marker for the given language.
func upsertTitleHashMarker(body, lang, hash string) string {
	return upsertTitleMarker(body, titleHashMarker(lang, hash), lang)
}

// upsertTitleSkipHashMarker replaces or appends a title skip hash marker for the given language.
func upsertTitleSkipHashMarker(body, lang, hash string) string {
	return upsertTitleMarker(body, titleSkipHashMarker(lang, hash), lang)
}

func upsertTitleMarker(body, newMarker, lang string) string {
	found := false
	lines := strings.Split(body, "\n")
	var result []string
	for _, line := range lines {
		if m := titleHashRe.FindStringSubmatch(line); m != nil && m[1] == lang {
			result = append(result, newMarker)
			found = true
			continue
		}
		result = append(result, line)
	}
	if !found {
		result = append(result, newMarker)
	}
	return strings.Join(result, "\n")
}

// ensureTitleOriginalMarker adds the subtitle-title-original marker if not present.
func ensureTitleOriginalMarker(body, originalTitle string) string {
	if titleOriginalRe.MatchString(body) {
		return body
	}
	marker := titleOriginalMarker(originalTitle)
	if body == "" {
		return marker
	}
	return body + "\n" + marker
}

// BuildTitle constructs a title string from the original title and translations.
// Format: "Original / ja翻訳 / ko번역" (sorted by language).
func BuildTitle(originalTitle string, translations map[string]string) string {
	if len(translations) == 0 {
		return originalTitle
	}

	langs := make([]string, 0, len(translations))
	for lang := range translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)

	parts := []string{originalTitle}
	for _, lang := range langs {
		if t := translations[lang]; t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, titleSeparator)
}

// CollectExistingTitleTranslations extracts existing title translations from the current title string.
// It parses title segments after the original title and matches them to languages in sorted order.
// Only non-empty translations (those that appear as segments in the title) are returned.
func CollectExistingTitleTranslations(body, currentTitle string) map[string]string {
	result := make(map[string]string)
	originalTitle := ExtractOriginalTitle(body, currentTitle)

	// Parse current title segments: "Original / trans1 / trans2"
	if !strings.HasPrefix(currentTitle, originalTitle+titleSeparator) {
		return result
	}
	rest := currentTitle[len(originalTitle)+len(titleSeparator):]
	segments := strings.Split(rest, titleSeparator)

	// Find all non-skip language markers in body, sorted.
	// Skip markers (same-language skip) don't produce title segments.
	var langs []string
	for _, m := range titleHashRe.FindAllStringSubmatch(body, -1) {
		if m[3] == " skip" {
			continue
		}
		langs = append(langs, m[1])
	}
	sort.Strings(langs)

	// Match segments to languages in sorted order.
	segIdx := 0
	for _, lang := range langs {
		if segIdx >= len(segments) {
			break
		}
		result[lang] = segments[segIdx]
		segIdx++
	}

	return result
}

// StripTitleMarkers removes all title-related markers from body.
func StripTitleMarkers(body string) string {
	lines := strings.Split(body, "\n")
	var result []string
	for _, line := range lines {
		if titleOriginalRe.MatchString(line) {
			continue
		}
		if titleHashRe.MatchString(line) {
			continue
		}
		result = append(result, line)
	}
	out := strings.Join(result, "\n")
	out = strings.TrimRight(out, "\n")
	return out
}

// StripTitleMarkersForLang removes title markers for a specific language from body.
func StripTitleMarkersForLang(body, lang string) string {
	lines := strings.Split(body, "\n")
	var result []string
	hasOtherLangs := false

	for _, line := range lines {
		if m := titleHashRe.FindStringSubmatch(line); m != nil {
			if m[1] == lang {
				continue
			}
			hasOtherLangs = true
		}
		result = append(result, line)
	}

	// If no other language markers remain, also remove the original marker
	if !hasOtherLangs {
		var filtered []string
		for _, line := range result {
			if titleOriginalRe.MatchString(line) {
				continue
			}
			filtered = append(filtered, line)
		}
		result = filtered
	}

	out := strings.Join(result, "\n")
	out = strings.TrimRight(out, "\n")
	return out
}
