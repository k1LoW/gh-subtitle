package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/k1LoW/gh-subtitle/internal/github"
	"github.com/k1LoW/gh-subtitle/internal/subtitle"
	"github.com/k1LoW/gh-subtitle/internal/translator"
	"github.com/k1LoW/gh-subtitle/version"
	"github.com/spf13/cobra"
)

var (
	translateLangs []string
	model          string
	dryRun         bool
	bodyOnly       bool
	clearMode      bool
	includeBots    bool
	byok           bool
	baseURL        string
	skipTitle      bool
)

var rootCmd = &cobra.Command{
	Use:     "gh-subtitle <URL>",
	Short:   "Translate GitHub PR/Issue/Discussion content and add subtitles",
	Version: version.Version,
	Args:    cobra.ExactArgs(1),
	RunE:    runRoot,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringSliceVarP(&translateLangs, "translate", "t", nil, "Target language(s) for translation (required, can be specified multiple times)")
	rootCmd.Flags().StringVarP(&model, "model", "m", "copilot:gpt-4o-mini", "Model to use in <provider>:<model_name> format (e.g. copilot:gpt-4o-mini, openai:gpt-4o with --byok)")
	rootCmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Show translations without updating GitHub")
	rootCmd.Flags().BoolVar(&bodyOnly, "body-only", false, "Translate only the body (skip comments)")
	rootCmd.Flags().BoolVar(&clearMode, "clear", false, "Remove translation marker blocks")
	rootCmd.Flags().BoolVar(&includeBots, "include-bots", false, "Include bot comments in translation (skipped by default)")
	rootCmd.Flags().BoolVar(&byok, "byok", false, "Use BYOK (Bring Your Own Key) mode with Copilot SDK (GH_SUBTITLE_PROVIDER_API_KEY required for openai/anthropic/azure)")
	rootCmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL for BYOK provider (env: GH_SUBTITLE_PROVIDER_BASE_URL)")
	rootCmd.Flags().BoolVar(&skipTitle, "skip-title", false, "Skip title translation")
}

func runRoot(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if !clearMode && len(translateLangs) == 0 {
		return fmt.Errorf("--translate (-t) is required")
	}
	for _, lang := range translateLangs {
		if utf8.RuneCountInString(lang) > 10 {
			return fmt.Errorf("language tag too long (max 10 characters): %q", lang)
		}
	}

	parsed, err := github.ParseURL(args[0])
	if err != nil {
		return err
	}

	items, err := github.FetchContent(parsed, bodyOnly)
	if err != nil {
		return err
	}

	if clearMode {
		return runClear(parsed, items)
	}

	return runTranslate(ctx, parsed, items)
}

func runClear(parsed *github.ParsedURL, items []github.ContentItem) error {
	for _, item := range items {
		if item.IsBot && !includeBots {
			continue
		}

		// Restore title if this is a body item with title
		if item.Title != "" && !skipTitle {
			originalTitle := subtitle.ExtractOriginalTitle(item.Body, item.Title)
			if originalTitle != item.Title {
				if dryRun {
					fmt.Fprintf(os.Stderr, "[dry-run] Would restore title for %s: %q\n", contentLabel(item), originalTitle)
				} else {
					if err := github.UpdateTitle(parsed, item, originalTitle); err != nil {
						if errors.Is(err, github.ErrValidationFailed) {
							fmt.Fprintf(os.Stderr, "Skipping title restore for %s (not editable)\n", contentLabel(item))
						} else {
							return fmt.Errorf("failed to restore title for %s: %w", contentLabel(item), err)
						}
					} else {
						fmt.Fprintf(os.Stderr, "Restored title for %s\n", contentLabel(item))
					}
				}
			}
		}

		var newBody string
		if len(translateLangs) == 0 {
			newBody = subtitle.StripTranslation(item.Body)
		} else {
			newBody = item.Body
			for _, lang := range translateLangs {
				newBody = subtitle.StripTranslationForLang(newBody, lang)
				newBody = subtitle.StripTitleMarkersForLang(newBody, lang)
			}
		}

		if newBody == item.Body {
			continue
		}

		if dryRun {
			fmt.Fprintf(os.Stderr, "[dry-run] Would clear translations for %s\n", contentLabel(item))
			continue
		}

		if err := github.UpdateContent(parsed, item, newBody); err != nil {
			if errors.Is(err, github.ErrValidationFailed) {
				fmt.Fprintf(os.Stderr, "Skipping %s (not editable)\n", contentLabel(item))
				continue
			}
			return fmt.Errorf("failed to update %s: %w", contentLabel(item), err)
		}
		fmt.Fprintf(os.Stderr, "Cleared translations for %s\n", contentLabel(item))
	}
	return nil
}

func runTranslate(ctx context.Context, parsed *github.ParsedURL, items []github.ContentItem) error {
	var trans translator.Translator
	var transErr error

	for _, lang := range translateLangs {
		// Collect items needing translation for this language
		var toTranslate []translator.TranslationInput
		var targetItems []github.ContentItem

		for _, item := range items {
			if item.IsBot && !includeBots {
				fmt.Fprintf(os.Stderr, "Skipping %s (bot)\n", contentLabel(item))
				continue
			}

			needsBody := subtitle.NeedsTranslation(item.Body, lang)
			needsTitle := !skipTitle && item.Title != "" && subtitle.NeedsTitleTranslation(item.Body, item.Title, lang)

			if !needsBody && !needsTitle {
				fmt.Fprintf(os.Stderr, "Skipping %s for %s (up to date)\n", contentLabel(item), lang)
				continue
			}

			if needsBody {
				original := subtitle.StripTranslation(item.Body)
				if original != "" {
					key := contentKey(item)
					toTranslate = append(toTranslate, translator.TranslationInput{
						Key:  key,
						Text: original,
					})
					targetItems = append(targetItems, item)
				}
			}

			if needsTitle {
				originalTitle := subtitle.ExtractOriginalTitle(item.Body, item.Title)
				titleKey := contentKey(item) + "_title"
				toTranslate = append(toTranslate, translator.TranslationInput{
					Key:  titleKey,
					Text: originalTitle,
				})
				if !needsBody {
					targetItems = append(targetItems, item)
				}
			}
		}

		if len(toTranslate) == 0 {
			continue
		}

		// Lazily initialize translator
		if trans == nil {
			provider, modelName, err := parseModel(model)
			if err != nil {
				return err
			}
			providerConfig, err := buildProviderConfig(provider, byok, baseURL)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Starting %s translator (model: %s)...\n", provider, modelName)
			trans, transErr = translator.NewCopilotTranslator(ctx, modelName, providerConfig)
			if transErr != nil {
				return fmt.Errorf("failed to create translator: %w", transErr)
			}
			defer trans.Close() //nolint:errcheck
		}

		fmt.Fprintf(os.Stderr, "Translating %d item(s) to %s...\n", len(toTranslate), lang)

		outputs, err := trans.TranslateBatch(ctx, lang, toTranslate)
		if err != nil {
			return fmt.Errorf("translation failed: %w", err)
		}

		// Build key->translation map
		translationOutputMap := make(map[string]translator.TranslationOutput)
		for _, out := range outputs {
			translationOutputMap[out.Key] = out
		}

		// Apply translations (deduplicate items)
		processed := make(map[string]bool)
		for _, item := range targetItems {
			itemID := contentKey(item)
			if processed[itemID] {
				continue
			}
			processed[itemID] = true

			bodyKey := contentKey(item)
			titleKey := bodyKey + "_title"

			bodyOut, hasBody := translationOutputMap[bodyKey]
			titleOut, hasTitle := translationOutputMap[titleKey]

			// Handle body skip (same language)
			bodySkip := hasBody && bodyOut.From != "" && bodyOut.From == bodyOut.To
			titleSkip := hasTitle && titleOut.From != "" && titleOut.From == titleOut.To

			newBody := item.Body

			// Apply body translation or skip marker
			if hasBody {
				if bodySkip {
					newBody = subtitle.ApplySkipMarker(newBody, lang)
					fmt.Fprintf(os.Stderr, "Skipping %s body for %s (already in %s)\n", contentLabel(item), lang, bodyOut.From)
				} else {
					var applyErr error
					newBody, applyErr = subtitle.ApplyTranslation(newBody, bodyOut.Text, lang, model)
					if applyErr != nil {
						return fmt.Errorf("failed to apply translation for %s: %w", contentLabel(item), applyErr)
					}
				}
			}

			// Apply title translation
			if hasTitle && item.Title != "" {
				originalTitle := subtitle.ExtractOriginalTitle(item.Body, item.Title)
				newBody = subtitle.PrepareTitleTranslation(newBody, originalTitle)

				if titleSkip {
					newBody = subtitle.ApplyTitleSkipMarker(newBody, originalTitle, lang)
					fmt.Fprintf(os.Stderr, "Skipping %s title for %s (already in %s)\n", contentLabel(item), lang, titleOut.From)
				} else {
					newBody = subtitle.ApplyTitleTranslation(newBody, lang, titleOut.Text)

					// Build the new title
					existingTranslations := subtitle.CollectExistingTitleTranslations(newBody, item.Title)
					existingTranslations[lang] = titleOut.Text
					newTitle := subtitle.BuildTitle(originalTitle, existingTranslations)

					if len(newTitle) > subtitle.GitHubMaxTitleLength {
						fmt.Fprintf(os.Stderr, "Warning: title too long (%d chars) for %s, skipping title translation for %s\n", len(newTitle), contentLabel(item), lang)
					} else if newTitle != item.Title {
						if dryRun {
							fmt.Fprintf(os.Stderr, "[dry-run] %s title (%s): %s\n", contentLabel(item), lang, newTitle)
						} else {
							if err := github.UpdateTitle(parsed, item, newTitle); err != nil {
								if errors.Is(err, github.ErrValidationFailed) {
									fmt.Fprintf(os.Stderr, "Skipping title for %s (not editable)\n", contentLabel(item))
								} else {
									return fmt.Errorf("failed to update title for %s: %w", contentLabel(item), err)
								}
							} else {
								fmt.Fprintf(os.Stderr, "Updated %s title with %s translation\n", contentLabel(item), lang)
								items = updateItemTitle(items, item, newTitle)
							}
						}
					}
				}
			}

			if newBody == item.Body {
				continue
			}

			if dryRun {
				fmt.Fprintf(os.Stderr, "[dry-run] %s (%s):\n", contentLabel(item), lang)
				fmt.Println(newBody)
				fmt.Println()
				continue
			}

			if err := github.UpdateContent(parsed, item, newBody); err != nil {
				if errors.Is(err, github.ErrValidationFailed) {
					fmt.Fprintf(os.Stderr, "Skipping %s (not editable)\n", contentLabel(item))
					continue
				}
				return fmt.Errorf("failed to update %s: %w", contentLabel(item), err)
			}
			fmt.Fprintf(os.Stderr, "Updated %s with %s translation\n", contentLabel(item), lang)

			items = updateItemBody(items, item, newBody)
		}
	}

	return nil
}

func updateItemBody(items []github.ContentItem, target github.ContentItem, newBody string) []github.ContentItem {
	for i, item := range items {
		if item.Type == target.Type && item.NodeID == target.NodeID && item.DatabaseID == target.DatabaseID && item.Number == target.Number {
			items[i].Body = newBody
			break
		}
	}
	return items
}

func updateItemTitle(items []github.ContentItem, target github.ContentItem, newTitle string) []github.ContentItem {
	for i, item := range items {
		if item.Type == target.Type && item.NodeID == target.NodeID && item.DatabaseID == target.DatabaseID && item.Number == target.Number {
			items[i].Title = newTitle
			break
		}
	}
	return items
}

func contentKey(item github.ContentItem) string {
	switch item.Type {
	case github.ContentTypePRBody, github.ContentTypeIssueBody:
		return "body"
	case github.ContentTypeDiscussionBody:
		return "dbody"
	case github.ContentTypeIssueComment:
		return fmt.Sprintf("ic_%d", item.DatabaseID)
	case github.ContentTypeReviewComment:
		return fmt.Sprintf("rc_%d", item.DatabaseID)
	case github.ContentTypeDiscussionComment:
		return fmt.Sprintf("dc_%s", item.NodeID)
	default:
		return "unknown"
	}
}

func parseModel(model string) (provider, modelName string, err error) {
	parts := strings.SplitN(model, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid --model format: %q (expected <provider>:<model_name>, e.g. copilot:gpt-4o-mini, openai:gpt-4o)", model)
	}
	return parts[0], parts[1], nil
}

func buildProviderConfig(provider string, byokMode bool, flagBaseURL string) (*copilot.ProviderConfig, error) {
	if !byokMode {
		if provider != "copilot" {
			return nil, fmt.Errorf("provider %q requires --byok flag", provider)
		}
		return nil, nil
	}

	if provider == "copilot" {
		return nil, fmt.Errorf("--byok cannot be used with copilot provider")
	}

	type providerDefaults struct {
		typ         string
		defaultURL  string
		keyRequired bool
	}

	known := map[string]providerDefaults{
		"openai":    {typ: "openai", defaultURL: "https://api.openai.com/v1", keyRequired: true},
		"anthropic": {typ: "anthropic", defaultURL: "https://api.anthropic.com", keyRequired: true},
		"azure":     {typ: "azure", defaultURL: "", keyRequired: true},
		"ollama":    {typ: "openai", defaultURL: "http://localhost:11434/v1", keyRequired: false},
	}

	defaults, ok := known[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported BYOK provider: %q (supported: openai, anthropic, azure, ollama)", provider)
	}

	apiKey := os.Getenv("GH_SUBTITLE_PROVIDER_API_KEY")
	if defaults.keyRequired && apiKey == "" {
		return nil, fmt.Errorf("GH_SUBTITLE_PROVIDER_API_KEY environment variable is required for provider %q", provider)
	}

	// Resolve base URL: --base-url flag > env var > provider default
	resolvedURL := flagBaseURL
	if resolvedURL == "" {
		resolvedURL = os.Getenv("GH_SUBTITLE_PROVIDER_BASE_URL")
	}
	if resolvedURL == "" {
		resolvedURL = defaults.defaultURL
	}
	if resolvedURL == "" {
		return nil, fmt.Errorf("--base-url or GH_SUBTITLE_PROVIDER_BASE_URL is required for provider %q", provider)
	}

	return &copilot.ProviderConfig{
		Type:    defaults.typ,
		BaseURL: resolvedURL,
		APIKey:  apiKey,
	}, nil
}

func contentLabel(item github.ContentItem) string {
	switch item.Type {
	case github.ContentTypePRBody:
		return fmt.Sprintf("PR #%d body", item.Number)
	case github.ContentTypeIssueBody:
		return fmt.Sprintf("Issue #%d body", item.Number)
	case github.ContentTypeDiscussionBody:
		return fmt.Sprintf("Discussion body (%s)", item.NodeID)
	case github.ContentTypeIssueComment:
		return fmt.Sprintf("issue comment #%d", item.DatabaseID)
	case github.ContentTypeReviewComment:
		return fmt.Sprintf("review comment #%d", item.DatabaseID)
	case github.ContentTypeDiscussionComment:
		return fmt.Sprintf("discussion comment (%s)", item.NodeID)
	default:
		return strconv.FormatInt(item.DatabaseID, 10)
	}
}
