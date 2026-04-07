package subtitle

import (
	"testing"
)

func TestStripTranslation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "no markers",
			body: "Hello world",
			want: "Hello world",
		},
		{
			name: "single language marker",
			body: "Hello world\n\n<!-- subtitle:ja:start sha256:abcd1234 -->\n---\nこんにちは世界\n\n---\n<sub>Translated</sub>\n<!-- subtitle:ja:end -->",
			want: "Hello world",
		},
		{
			name: "multiple language markers",
			body: "Hello world\n\n<!-- subtitle:ja:start sha256:abcd1234 -->\n日本語\n<!-- subtitle:ja:end -->\n\n<!-- subtitle:en:start sha256:abcd1234 -->\nEnglish\n<!-- subtitle:en:end -->",
			want: "Hello world",
		},
		{
			name: "empty body",
			body: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripTranslation(tt.body)
			if got != tt.want {
				t.Errorf("StripTranslation() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripTranslationForLang(t *testing.T) {
	body := "Hello world\n\n<!-- subtitle:ja:start sha256:abcd1234 -->\n日本語\n<!-- subtitle:ja:end -->\n\n<!-- subtitle:en:start sha256:abcd1234 -->\nEnglish\n<!-- subtitle:en:end -->"

	// Strip only ja
	got := StripTranslationForLang(body, "ja")
	if got == body {
		t.Error("StripTranslationForLang(ja) should have changed the body")
	}
	if !contains(got, "<!-- subtitle:en:start") {
		t.Error("StripTranslationForLang(ja) should preserve en markers")
	}
	if contains(got, "<!-- subtitle:ja:start") {
		t.Error("StripTranslationForLang(ja) should remove ja markers")
	}
}

func TestNeedsTranslation(t *testing.T) {
	original := "Hello world"
	hash := computeHash(original)

	tests := []struct {
		name string
		body string
		lang string
		want bool
	}{
		{
			name: "no marker - needs translation",
			body: original,
			lang: "ja",
			want: true,
		},
		{
			name: "marker with matching hash - no translation needed",
			body: original + "\n\n<!-- subtitle:ja:start sha256:" + hash + " -->\n翻訳\n<!-- subtitle:ja:end -->",
			lang: "ja",
			want: false,
		},
		{
			name: "marker with different hash - needs translation",
			body: original + "\n\n<!-- subtitle:ja:start sha256:00000000 -->\n古い翻訳\n<!-- subtitle:ja:end -->",
			lang: "ja",
			want: true,
		},
		{
			name: "different language marker - needs translation",
			body: original + "\n\n<!-- subtitle:en:start sha256:" + hash + " -->\nTranslation\n<!-- subtitle:en:end -->",
			lang: "ja",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsTranslation(tt.body, tt.lang)
			if got != tt.want {
				t.Errorf("NeedsTranslation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyTranslation(t *testing.T) {
	t.Run("new translation", func(t *testing.T) {
		body := "Hello world"
		got, err := ApplyTranslation(body, "こんにちは世界", "ja", "gpt-4o-mini")
		if err != nil {
			t.Fatalf("ApplyTranslation() error = %v", err)
		}

		if !contains(got, "<!-- subtitle:ja:start sha256:") {
			t.Error("should contain start marker")
		}
		if !contains(got, "<!-- subtitle:ja:end -->") {
			t.Error("should contain end marker")
		}
		if !contains(got, "こんにちは世界") {
			t.Error("should contain translation")
		}
		if !contains(got, "Hello world") {
			t.Error("should contain original text")
		}
		if !contains(got, "(model: gpt-4o-mini)") {
			t.Error("should contain model name")
		}
	})

	t.Run("replace existing translation", func(t *testing.T) {
		original := "Hello world"
		hash := computeHash(original)
		body := original + "\n\n<!-- subtitle:ja:start sha256:" + hash + " -->\n---\n古い翻訳\n\n---\n<sub>Translated</sub>\n<!-- subtitle:ja:end -->"

		got, err := ApplyTranslation(body, "新しい翻訳", "ja", "gpt-4o-mini")
		if err != nil {
			t.Fatalf("ApplyTranslation() error = %v", err)
		}

		if !contains(got, "新しい翻訳") {
			t.Error("should contain new translation")
		}
		if contains(got, "古い翻訳") {
			t.Error("should not contain old translation")
		}
	})

	t.Run("multiple languages", func(t *testing.T) {
		body := "Hello world"

		// Add Japanese
		body, err := ApplyTranslation(body, "こんにちは世界", "ja", "gpt-4o-mini")
		if err != nil {
			t.Fatalf("ApplyTranslation(ja) error = %v", err)
		}

		// Add English (shouldn't affect Japanese)
		body, err = ApplyTranslation(body, "Hola mundo", "es", "gpt-4o-mini")
		if err != nil {
			t.Fatalf("ApplyTranslation(es) error = %v", err)
		}

		if !contains(body, "<!-- subtitle:ja:start") {
			t.Error("should contain ja marker")
		}
		if !contains(body, "<!-- subtitle:es:start") {
			t.Error("should contain es marker")
		}
		if !contains(body, "こんにちは世界") {
			t.Error("should contain Japanese translation")
		}
		if !contains(body, "Hola mundo") {
			t.Error("should contain Spanish translation")
		}
	})
}

func TestComputeHash(t *testing.T) {
	h := computeHash("Hello world")
	if len(h) != 8 {
		t.Errorf("hash length = %d, want 8", len(h))
	}

	// Same input should produce same hash
	h2 := computeHash("Hello world")
	if h != h2 {
		t.Errorf("hash mismatch for same input: %s != %s", h, h2)
	}

	// Different input should produce different hash
	h3 := computeHash("Different text")
	if h == h3 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestIdempotency(t *testing.T) {
	body := "Hello world"

	// Apply translation
	body1, err := ApplyTranslation(body, "こんにちは世界", "ja", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("first ApplyTranslation() error = %v", err)
	}

	// Should not need translation now
	if NeedsTranslation(body1, "ja") {
		t.Error("should not need translation after applying")
	}
}

// --- Title translation tests ---

func TestNeedsTitleTranslation(t *testing.T) {
	title := "Fix bug in parser"
	hash := computeHash(title)

	tests := []struct {
		name  string
		body  string
		title string
		lang  string
		want  bool
	}{
		{
			name:  "no marker - needs translation",
			body:  "Some body text",
			title: title,
			lang:  "ja",
			want:  true,
		},
		{
			name:  "matching hash - no translation needed",
			body:  "Some body text\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", hash),
			title: title + " / バグ修正",
			lang:  "ja",
			want:  false,
		},
		{
			name:  "different hash - needs translation (title changed)",
			body:  "Some body text\n" + titleOriginalMarker("Old title") + "\n" + titleHashMarker("ja", computeHash("Old title")),
			title: "New title",
			lang:  "ja",
			want:  true,
		},
		{
			name:  "different language - needs translation",
			body:  "Some body text\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ko", hash),
			title: title,
			lang:  "ja",
			want:  true,
		},
		{
			name:  "empty title - no translation needed",
			body:  "Some body text",
			title: "",
			lang:  "ja",
			want:  false,
		},
		{
			name:  "title with separator externally edited - needs translation",
			body:  "Some body text\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", hash),
			title: "Edited title / バグ修正",
			lang:  "ja",
			want:  true,
		},
		{
			name:  "title without separator externally edited - needs translation",
			body:  "Some body text\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", hash),
			title: "Completely different title",
			lang:  "ja",
			want:  true,
		},
		{
			name:  "title with separator but no markers (first run) - needs translation",
			body:  "Some body text",
			title: "Fix path / to file",
			lang:  "ja",
			want:  true,
		},
		{
			name:  "markers exist but translated segment externally removed - needs translation",
			body:  "Some body text\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", hash),
			title: title,
			lang:  "ja",
			want:  true,
		},
		{
			name:  "skip marker exists and title has no separator - no translation needed",
			body:  "Some body text\n" + titleOriginalMarker(title) + "\n" + titleSkipHashMarker("ja", hash),
			title: title,
			lang:  "ja",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsTitleTranslation(tt.body, tt.title, tt.lang)
			if got != tt.want {
				t.Errorf("NeedsTitleTranslation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractOriginalTitle(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		title string
		want  string
	}{
		{
			name:  "from marker",
			body:  "body\n" + titleOriginalMarker("Original Title"),
			title: "Original Title / 翻訳タイトル",
			want:  "Original Title",
		},
		{
			name:  "from title with separator (with hash marker)",
			body:  "body\n" + titleHashMarker("ja", computeHash("Original Title")),
			title: "Original Title / 翻訳タイトル",
			want:  "Original Title",
		},
		{
			name:  "title with separator but no markers (first run)",
			body:  "body without markers",
			title: "Fix path / to file",
			want:  "Fix path / to file",
		},
		{
			name:  "plain title no separator",
			body:  "body without markers",
			title: "Just a title",
			want:  "Just a title",
		},
		{
			name:  "title with slash in original (marker present)",
			body:  "body\n" + titleOriginalMarker("Fix path / to file"),
			title: "Fix path / to file / パス修正",
			want:  "Fix path / to file",
		},
		{
			name:  "externally edited title (stored original differs from current segment)",
			body:  "body\n" + titleOriginalMarker("Old Title") + "\n" + titleHashMarker("ja", computeHash("Old Title")),
			title: "New Title / 古い翻訳",
			want:  "New Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractOriginalTitle(tt.body, tt.title)
			if got != tt.want {
				t.Errorf("ExtractOriginalTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildTitle(t *testing.T) {
	tests := []struct {
		name         string
		original     string
		translations map[string]string
		want         string
	}{
		{
			name:         "single language",
			original:     "Fix bug",
			translations: map[string]string{"ja": "バグ修正"},
			want:         "Fix bug / バグ修正",
		},
		{
			name:         "multiple languages sorted",
			original:     "Fix bug",
			translations: map[string]string{"ko": "버그 수정", "ja": "バグ修正"},
			want:         "Fix bug / バグ修正 / 버그 수정",
		},
		{
			name:         "no translations",
			original:     "Fix bug",
			translations: map[string]string{},
			want:         "Fix bug",
		},
		{
			name:         "skip empty translations",
			original:     "Fix bug",
			translations: map[string]string{"ja": "バグ修正", "ko": ""},
			want:         "Fix bug / バグ修正",
		},
		{
			name:         "skip translation identical to original",
			original:     "バグ修正",
			translations: map[string]string{"en": "Bug fix", "ja": "バグ修正"},
			want:         "バグ修正 / Bug fix",
		},
		{
			name:         "all translations identical to original",
			original:     "Fix bug",
			translations: map[string]string{"en": "Fix bug"},
			want:         "Fix bug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTitle(tt.original, tt.translations)
			if got != tt.want {
				t.Errorf("BuildTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyTitleTranslation(t *testing.T) {
	title := "Fix bug"
	body := "Some body\n" + titleOriginalMarker(title)

	// Apply first language
	body = ApplyTitleTranslation(body, "ja", title)
	if !contains(body, "<!-- subtitle-title:ja sha256:") {
		t.Error("should contain ja title marker")
	}

	// Apply second language
	body = ApplyTitleTranslation(body, "ko", title)
	if !contains(body, "<!-- subtitle-title:ko sha256:") {
		t.Error("should contain ko title marker")
	}
	if !contains(body, "<!-- subtitle-title:ja sha256:") {
		t.Error("should still contain ja title marker")
	}
}

func TestStripTitleMarkers(t *testing.T) {
	title := "Fix bug"
	hash := computeHash(title)
	body := "Some body\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", hash)

	got := StripTitleMarkers(body)
	if contains(got, "subtitle-title") {
		t.Error("should not contain any title markers")
	}
	if !contains(got, "Some body") {
		t.Error("should preserve original body content")
	}
}

func TestStripTitleMarkersForLang(t *testing.T) {
	title := "Fix bug"
	hash := computeHash(title)
	body := "Some body\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", hash) + "\n" + titleHashMarker("ko", hash)

	// Strip only ja
	got := StripTitleMarkersForLang(body, "ja")
	if contains(got, "subtitle-title:ja") {
		t.Error("should not contain ja title marker")
	}
	if !contains(got, "subtitle-title:ko") {
		t.Error("should preserve ko title marker")
	}
	if !contains(got, "subtitle-title-original") {
		t.Error("should preserve original marker when other langs exist")
	}

	// Strip ko too - should also remove original marker
	got = StripTitleMarkersForLang(got, "ko")
	if contains(got, "subtitle-title") {
		t.Error("should not contain any title markers when all langs removed")
	}
}

func TestCollectExistingTitleTranslations(t *testing.T) {
	title := "Fix bug"
	hash := computeHash(title)
	body := "Some body\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", hash) + "\n" + titleHashMarker("ko", hash)
	currentTitle := "Fix bug / バグ修正 / 버그 수정"

	got := CollectExistingTitleTranslations(body, currentTitle)
	if got["ja"] != "バグ修正" {
		t.Errorf("ja translation = %q, want %q", got["ja"], "バグ修正")
	}
	if got["ko"] != "버그 수정" {
		t.Errorf("ko translation = %q, want %q", got["ko"], "버그 수정")
	}
}

func TestCollectExistingTitleTranslationsWithSkipMarker(t *testing.T) {
	title := "Fix bug"
	hash := computeHash(title)
	// "en" has a skip marker (same-language), "ja" has a normal marker
	body := "Some body\n" + titleOriginalMarker(title) + "\n" + titleSkipHashMarker("en", hash) + "\n" + titleHashMarker("ja", hash)
	currentTitle := "Fix bug / バグ修正"

	got := CollectExistingTitleTranslations(body, currentTitle)
	if got["ja"] != "バグ修正" {
		t.Errorf("ja translation = %q, want %q", got["ja"], "バグ修正")
	}
	if _, ok := got["en"]; ok {
		t.Errorf("en should not have a translation (skip marker), but got %q", got["en"])
	}
}

func TestExternalEditIdempotency(t *testing.T) {
	// Simulate: original title "Old Title" was translated, then user externally edits to "New Title".
	oldTitle := "Old Title"
	oldHash := computeHash(oldTitle)
	body := "Some body\n" + titleOriginalMarker(oldTitle) + "\n" + titleHashMarker("ja", oldHash)
	currentTitle := "New Title / 古い翻訳"

	// NeedsTitleTranslation should detect the external edit.
	if !NeedsTitleTranslation(body, currentTitle, "ja") {
		t.Fatal("should need re-translation after external edit")
	}

	// Simulate re-translation: apply new title translation.
	newOriginal := ExtractOriginalTitle(body, currentTitle)
	if newOriginal != "New Title" {
		t.Fatalf("ExtractOriginalTitle() = %q, want %q", newOriginal, "New Title")
	}

	body = ApplyTitleTranslation(body, "ja", newOriginal)

	// After applying, the original marker should be updated to the new title.
	stored := parseTitleOriginal(body)
	if stored != "New Title" {
		t.Errorf("parseTitleOriginal() = %q, want %q after upsert", stored, "New Title")
	}

	// Now NeedsTitleTranslation should return false (idempotent).
	newTitle := BuildTitle(newOriginal, map[string]string{"ja": "新しい翻訳"})
	if NeedsTitleTranslation(body, newTitle, "ja") {
		t.Error("should not need translation after applying (idempotency broken)")
	}
}

func TestStripTranslationWithTitleMarkers(t *testing.T) {
	title := "Fix bug"
	body := "Hello world\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", computeHash(title))

	got := StripTranslation(body)
	if got != "Hello world" {
		t.Errorf("StripTranslation() = %q, want %q", got, "Hello world")
	}
}

func TestNeedsTranslationWithTitleMarkers(t *testing.T) {
	original := "Hello world"
	title := "Fix bug"
	hash := computeHash(original)

	// Body with title markers + body translation marker
	body := original + "\n" + titleOriginalMarker(title) + "\n" + titleHashMarker("ja", computeHash(title)) + "\n\n<!-- subtitle:ja:start sha256:" + hash + " -->\n翻訳\n<!-- subtitle:ja:end -->"

	if NeedsTranslation(body, "ja") {
		t.Error("should not need body translation when hash matches (title markers should be stripped for hash computation)")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
