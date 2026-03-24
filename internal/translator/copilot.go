package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	copilot "github.com/github/copilot-sdk/go"
)

const minCopilotVersion = "0.0.411"

// CopilotTranslator implements Translator using the Copilot SDK.
type CopilotTranslator struct {
	client   *copilot.Client
	model    string
	provider *copilot.ProviderConfig
}

// NewCopilotTranslator creates a new CopilotTranslator.
// When provider is non-nil, BYOK mode is used and the provider config is passed to session creation.
func NewCopilotTranslator(ctx context.Context, model string, provider *copilot.ProviderConfig) (*CopilotTranslator, error) {
	if err := checkCopilotCLI(); err != nil {
		return nil, err
	}

	opts := &copilot.ClientOptions{
		LogLevel: "error",
	}
	// Follow the same token resolution order as the gh CLI: GH_TOKEN > GITHUB_TOKEN > gh auth
	if token := os.Getenv("GH_TOKEN"); token != "" {
		opts.GitHubToken = token
	} else if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		opts.GitHubToken = token
	}

	client := copilot.NewClient(opts)

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start copilot client: %w", err)
	}

	return &CopilotTranslator{
		client:   client,
		model:    model,
		provider: provider,
	}, nil
}

// TranslateBatch translates a batch of items to the target language.
func (t *CopilotTranslator) TranslateBatch(ctx context.Context, lang string, items []TranslationInput) ([]TranslationOutput, error) {
	if len(items) == 0 {
		return nil, nil
	}

	systemPrompt := fmt.Sprintf(`You are a translator. You will receive a JSON array of objects, each with "key" and "text" fields.
Translate all "text" values to %s and return a JSON array with the same structure.

Each output object must have: "key", "text" (translated), "from" (detected source language code, e.g. "en", "ja", "zh"), and "to" (target language code).

Rules:
- Preserve all Markdown formatting, links, images, code blocks, and HTML tags exactly.
- Do not translate content inside code blocks or inline code.
- Do not translate URLs, image paths, GitHub @mentions, or #references.
- Output ONLY the JSON array. No explanation, no markdown fences.
- If a text is already in the target language, return it unchanged.
- For the "from" field, detect the primary language based on the substantive prose written by the author (explanations, descriptions, comments), not boilerplate or structural elements. Ignore bilingual template headers, form labels, fixed-choice options, code blocks, and other structural elements that may appear in a different language.
- IMPORTANT: When text mixes technical English terms (e.g. library names, API names, component names) with non-English grammar and particles, detect the language based on the grammatical structure, NOT the technical terms. For example, "React Router の設定方法が分からない" is Japanese despite containing English technical terms — it MUST be fully translated to the target language (e.g. "I don't understand how to configure React Router"). Always translate the full sentence; never return the original text when the target language differs from the detected source language.`, lang)

	session, err := t.client.CreateSession(ctx, &copilot.SessionConfig{
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
		Model:               t.model,
		Provider:            t.provider,
		SystemMessage: &copilot.SystemMessageConfig{
			Content: systemPrompt,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create copilot session: %w", err)
	}
	defer session.Disconnect() //nolint:errcheck

	inputJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	var responseContent string
	var eventErr error
	done := make(chan struct{})

	unsubscribe := session.On(func(event copilot.SessionEvent) {
		switch event.Type {
		case "assistant.message":
			if event.Data.Content != nil {
				responseContent = *event.Data.Content
			}
		case "session.idle":
			close(done)
		case "error":
			if event.Data.Content != nil {
				eventErr = fmt.Errorf("copilot error: %s", *event.Data.Content)
			}
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})
	defer unsubscribe()

	_, err = session.Send(ctx, copilot.MessageOptions{
		Prompt: string(inputJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send message to copilot: %w", err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if eventErr != nil {
		return nil, eventErr
	}

	var outputs []TranslationOutput
	if err := json.Unmarshal([]byte(responseContent), &outputs); err != nil {
		return nil, fmt.Errorf("failed to parse copilot response: %w\nresponse: %s", err, responseContent)
	}

	return outputs, nil
}

// Close stops the Copilot client.
func (t *CopilotTranslator) Close() error {
	if t.client != nil {
		return t.client.Stop()
	}
	return nil
}

func checkCopilotCLI() error {
	out, err := exec.Command("copilot", "--version").Output()
	if err != nil {
		return fmt.Errorf("copilot CLI not found. Please install GitHub Copilot CLI >= %s", minCopilotVersion)
	}

	version := parseCopilotVersion(string(out))
	if version == "" {
		return fmt.Errorf("could not parse copilot CLI version from: %s", strings.TrimSpace(string(out)))
	}

	if compareVersions(version, minCopilotVersion) < 0 {
		return fmt.Errorf("copilot CLI version %s is too old. Please update to >= %s (run: copilot update)", version, minCopilotVersion)
	}

	return nil
}

var versionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

func parseCopilotVersion(output string) string {
	m := versionRe.FindString(output)
	return m
}


func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	for i := range 3 {
		var va, vb int
		if i < len(partsA) {
			va, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			vb, _ = strconv.Atoi(partsB[i])
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}
