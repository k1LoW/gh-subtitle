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

### Local

By default, gh-subtitle uses the logged-in user's authentication via the `gh` CLI (i.e., `gh auth login`). No additional configuration is needed.

### GitHub Actions / CI

For CI environments, set a token via environment variables:

| Environment Variable | Description |
|---|---|
| `GH_SUBTITLE_COPILOT_TOKEN` | Token for Copilot SDK authentication (highest priority) |
| `GITHUB_TOKEN` | Fallback token for Copilot SDK authentication |

Token resolution order: `GH_SUBTITLE_COPILOT_TOKEN` → `GITHUB_TOKEN` → logged-in user (via `gh auth`).

Note: The `gh` CLI uses `GH_TOKEN` or `GITHUB_TOKEN` for GitHub API operations independently.

```yaml
# Example GitHub Actions step
- run: gh subtitle ${{ github.event.pull_request.html_url }} -t ja
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    GH_SUBTITLE_COPILOT_TOKEN: ${{ secrets.COPILOT_TOKEN }}
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

## Contributing

To use this project from source, instead of a release:

    go build .
    gh extension remove subtitle
    gh extension install .
