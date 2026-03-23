# 🌐 gh-subtitle

`gh-subtitle` is a GitHub CLI (`gh`) extension that translates PR / Issue / Discussion bodies and comments, appending translated text as subtitles.

It uses the [Copilot SDK](https://github.com/github/copilot-sdk) to translate content and manages translation blocks with HTML comment markers for idempotent updates.

> [!WARNING]
> This tool sends PR/Issue/Discussion bodies and comments to an LLM for translation. Since these texts are user-generated content, they may contain prompt injection attempts that could cause the LLM to produce unexpected output. The translated output is written back to GitHub without content verification. Be aware of this risk when translating content from untrusted sources.

## Usage

```bash
# Add Japanese translation to a PR
$ gh subtitle https://github.com/owner/repo/pull/123 -t ja

# Add English translation to an Issue
$ gh subtitle https://github.com/owner/repo/issues/456 -t en

# Add both Japanese and English translations (mutual translation)
$ gh subtitle https://github.com/owner/repo/pull/123 -t ja -t en

# Translate only the body (skip comments)
$ gh subtitle https://github.com/owner/repo/pull/123 -t ja --body-only

# Preview translations without updating GitHub
$ gh subtitle https://github.com/owner/repo/pull/123 -t ja --dry-run

# Remove all translation blocks
$ gh subtitle https://github.com/owner/repo/pull/123 --clear

# Remove only Japanese translation blocks
$ gh subtitle https://github.com/owner/repo/pull/123 --clear -t ja

# Use a different model
$ gh subtitle https://github.com/owner/repo/pull/123 -t ja -m copilot:gpt-4o

# BYOK: Use OpenAI directly
$ GH_SUBTITLE_PROVIDER_API_KEY=sk-... gh subtitle https://github.com/owner/repo/pull/123 -t ja --byok -m openai:gpt-4o

# BYOK: Use Anthropic directly
$ GH_SUBTITLE_PROVIDER_API_KEY=sk-ant-... gh subtitle https://github.com/owner/repo/pull/123 -t ja --byok -m anthropic:claude-sonnet-4-20250514

# BYOK: Use Ollama (local, no API key needed)
$ gh subtitle https://github.com/owner/repo/pull/123 -t ja --byok -m ollama:llama3

# BYOK: Use Azure OpenAI (base URL required)
$ GH_SUBTITLE_PROVIDER_API_KEY=... gh subtitle https://github.com/owner/repo/pull/123 -t ja --byok -m azure:gpt-4 --base-url https://myinstance.openai.azure.com
```

### Supported Content

| Type | Body | Comments |
|------|------|----------|
| Pull Request | ✓ | Issue comments, Review comments |
| Issue | ✓ | Issue comments |
| Discussion | ✓ | Comments, Replies |

### Translation Markers

Translations are inserted at the end of each content item, wrapped in HTML comment markers:

```markdown
Original content...

<!-- subtitle:ja:start sha256:a1b2c3d4 -->
---
<sub>🌐 Translated by [gh-subtitle](https://github.com/k1LoW/gh-subtitle) (model: copilot:gpt-4o-mini)</sub>

日本語翻訳...
<!-- subtitle:ja:end -->
```

- **Language-scoped markers** — Each language gets its own marker block (`<!-- subtitle:ja:... -->`, `<!-- subtitle:en:... -->`), allowing multiple translations to coexist.
- **Hash-based change detection** — A SHA256 hash of the original text is embedded in the start marker. Re-running the command skips items whose original text hasn't changed.
- **Idempotent updates** — Running the same command multiple times is safe; existing translations are updated in place rather than duplicated.

## Install

```bash
$ gh extension install k1LoW/gh-subtitle
```

## Prerequisites

- [GitHub Copilot CLI](https://docs.github.com/en/copilot) >= 0.0.411 (`copilot --version` to check, `copilot update` to upgrade)

## Authentication

### Copilot Mode (default)

By default, gh-subtitle uses the Copilot SDK with the logged-in user's authentication via the `gh` CLI (i.e., `gh auth login`). No additional configuration is needed.

For CI environments, set `GITHUB_TOKEN` for Copilot SDK authentication.

Token resolution order: `GITHUB_TOKEN` → logged-in user (via `gh auth`).

Note: The `gh` CLI uses `GH_TOKEN` or `GITHUB_TOKEN` for GitHub API operations independently.

```yaml
# Example GitHub Actions step (Copilot mode)
- run: gh subtitle ${{ github.event.pull_request.html_url }} -t ja
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### BYOK Mode (`--byok`)

BYOK (Bring Your Own Key) mode allows you to use external LLM providers directly via the Copilot SDK's BYOK support. This is useful when Copilot's default authentication is not available (e.g., with GitHub App installation tokens).

| Environment Variable | Description |
|---|---|
| `GH_SUBTITLE_PROVIDER_API_KEY` | API key for the BYOK provider (required for `openai`, `anthropic`, `azure`) |
| `GH_SUBTITLE_PROVIDER_BASE_URL` | Base URL override for the BYOK provider |

Supported providers:

| Provider | Type | Default Base URL | API Key |
|----------|------|-----------------|---------|
| `openai` | `openai` | `https://api.openai.com/v1` | Required |
| `anthropic` | `anthropic` | `https://api.anthropic.com` | Required |
| `azure` | `azure` | *(none — must be specified)* | Required |
| `ollama` | `openai` | `http://localhost:11434/v1` | Not required |

Base URL resolution order: `--base-url` flag → `GH_SUBTITLE_PROVIDER_BASE_URL` env var → provider default.

Note: `GH_TOKEN` is for `gh` CLI API operations. Translation itself uses `GH_SUBTITLE_PROVIDER_API_KEY` for the configured provider.

```yaml
# Example GitHub Actions step (BYOK with OpenAI)
- run: gh subtitle ${{ github.event.pull_request.html_url }} -t ja --byok -m openai:gpt-4o
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    GH_SUBTITLE_PROVIDER_API_KEY: ${{ secrets.OPENAI_API_KEY }}
```

## Command Line Options

| Option | Short | Description |
|--------|-------|-------------|
| `--translate` | `-t` | Target language(s) for translation (required, can be specified multiple times) |
| `--model` | `-m` | Model to use in `<provider>:<model_name>` format (default: `copilot:gpt-4o-mini`) |
| `--dry-run` | `-n` | Show translations without updating GitHub |
| `--body-only` | | Translate only the body (skip comments) |
| `--clear` | | Remove translation marker blocks (all languages, or specific languages with `-t`) |
| `--include-bots` | | Include bot comments in translation (skipped by default) |
| `--byok` | | Use BYOK mode with an external LLM provider (`GH_SUBTITLE_PROVIDER_API_KEY` required for openai/anthropic/azure) |
| `--base-url` | | Base URL for BYOK provider (env: `GH_SUBTITLE_PROVIDER_BASE_URL`) |

## Contributing

To use this project from source, instead of a release:

    go build .
    gh extension remove subtitle
    gh extension install .
