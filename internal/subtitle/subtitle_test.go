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
