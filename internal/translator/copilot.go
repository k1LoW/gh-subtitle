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
	"github.com/github/copilot-sdk/go/rpc"
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
		// copilot.ModeEmpty is not used because it sets COPILOT_DISABLE_KEYTAR=1,
		// which breaks logins stored in the OS keychain. Its hardening is applied
		// per session in TranslateBatch instead.
	}
	opts.GitHubToken = resolveCopilotToken()

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

// denyAllPermissions rejects every permission request.
// Translation never needs tools, so any request is either a runtime surprise or
// the result of prompt injection in the untrusted input.
func denyAllPermissions(_ copilot.PermissionRequest, _ copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
	return &rpc.PermissionDecisionReject{}, nil
}

// newSessionConfig builds a session config for translating untrusted text
// (anyone can write a PR/Issue/comment). It mirrors the safe defaults of
// copilot.ModeEmpty without enabling that mode on the client.
func newSessionConfig(model string, provider *copilot.ProviderConfig, systemPrompt string) *copilot.SessionConfig {
	return &copilot.SessionConfig{
		// An empty non-nil slice is sent as [] and disables every tool; nil would
		// fall back to the default tool set.
		AvailableTools:      []string{},
		OnPermissionRequest: denyAllPermissions,
		Model:               model,
		Provider:            provider,
		SystemMessage: &copilot.SystemMessageConfig{
			Mode:    "customize",
			Content: systemPrompt,
			Sections: map[string]copilot.SectionOverride{
				"environment_context": {Action: copilot.SectionActionRemove},
			},
		},
		SkipCustomInstructions:             new(true),
		EnableOnDemandInstructionDiscovery: new(false),
		EnableSkills:                       new(false),
		EnableFileHooks:                    new(false),
		EnableHostGitOperations:            new(false),
		EnableSessionStore:                 new(false),
		EnableExperimentalMode:             new(false),
		CustomAgentsLocalOnly:              new(true),
		Memory:                             &copilot.MemoryConfiguration{Enabled: false},
	}
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
- Do not escape or modify any characters inside code blocks or inline code. For example, keep ` + "`</>`, `<div>`, `</Component>`" + ` as-is — never convert them to HTML entities like ` + "`&lt;/&gt;`" + `.
- Do not translate URLs, image paths, GitHub @mentions, or #references.
- Output ONLY the JSON array. No explanation, no markdown fences.
- CRITICAL: If a text is already in the target language, return the "text" field with the EXACT original string, byte-for-byte. Do NOT rephrase, paraphrase, reformat, or "clean up" the text. The output must be identical to the input.
- For the "from" field, detect the primary language based on the grammatical structure (particles, conjugations, word order, inflections), NOT based on vocabulary or technical terms. Ignore boilerplate, bilingual template headers, form labels, fixed-choice options, code blocks, and other structural elements.
- IMPORTANT: When text mixes technical English terms (e.g. library names, API names, component names, variable names) with non-English grammar, detect the language based on grammatical elements. For example:
  - "DBのindexが正しくsetupされていない" is Japanese (particles の, が and verb されていない) — "from" MUST be "ja".
  - "React Router の設定方法が分からない" is Japanese — "from" MUST be "ja".
  - Even very short text: if it contains particles like の, に, が, は, を, で, も, or verb endings like する, した, ない, ます, ている, the language is Japanese regardless of how many English/technical terms are present.
  When the detected source language differs from the target, translate the full sentence. When they match, return the text unchanged (see CRITICAL rule above).`, lang)

	session, err := t.client.CreateSession(ctx, newSessionConfig(t.model, t.provider, systemPrompt))
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
		switch d := event.Data.(type) {
		case *copilot.AssistantMessageData:
			responseContent = d.Content
		case *copilot.SessionIdleData:
			close(done)
		case *copilot.SessionErrorData:
			eventErr = fmt.Errorf("copilot error: %s", d.Message)
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
	if err := json.Unmarshal([]byte(stripCodeFence(responseContent)), &outputs); err != nil {
		return nil, fmt.Errorf("failed to parse copilot response: %w\nresponse: %s", err, responseContent)
	}

	return outputs, nil
}

// stripCodeFence removes a markdown code fence wrapping the whole response.
// The system prompt already forbids fences, but some models (e.g. claude-haiku-4.5)
// add them anyway, and the usable models in Copilot mode are decided by org policy,
// so the parser has to tolerate them instead of relying on the prompt.
func stripCodeFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "```") || !strings.HasSuffix(trimmed, "```") {
		return s
	}
	first := strings.IndexByte(trimmed, '\n')
	if first < 0 {
		return s
	}
	last := strings.LastIndexByte(trimmed, '\n')
	if last <= first {
		return s
	}
	return strings.TrimSpace(trimmed[first+1 : last])
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

// resolveCopilotToken returns the token for Copilot requests, or an empty string to use the logged-in user.
// COPILOT_GITHUB_TOKEN takes precedence so that Copilot requests and gh CLI operations can use different tokens,
// e.g. the Actions GITHUB_TOKEN for Copilot and a fine-grained PAT for editing Discussions, which GITHUB_TOKEN cannot do.
// The name follows the variable Copilot CLI already reads, instead of a gh-subtitle specific one.
func resolveCopilotToken() string {
	for _, key := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if token := os.Getenv(key); token != "" {
			return token
		}
	}
	return ""
}
