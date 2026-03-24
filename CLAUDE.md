# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gh-subtitle is a GitHub CLI (`gh`) extension that translates PR/Issue/Discussion titles, bodies, and comments, appending translations as "subtitles" using idempotent HTML comment markers. It uses the GitHub Copilot SDK for translation.

## Common Commands

```bash
# Build
go build .

# Run tests with coverage
make test

# Lint (golangci-lint with errorlint, godot, gosec, misspell, revive, modernize, etc.)
make lint

# Run a single test
go test ./internal/subtitle/ -run TestApplyTranslation

# CI pipeline (depsdev + test)
make ci
```

## Architecture

The codebase follows a pipeline: **CLI parsing → GitHub fetch → translation → subtitle marker management → GitHub update**.

- **`cmd/root.go`** — CLI entry point and orchestration. Dispatches to `runTranslate()` or `runClear()`. Lazily initializes translator, iterates over target languages, batches items for translation. Handles title translation alongside body translation.
- **`internal/github/`** — GitHub API interactions via `gh` CLI commands. Uses REST API for PRs/Issues, GraphQL for Discussions. `ParseURL()` determines content type from URL.
- **`internal/translator/`** — `Translator` interface with `CopilotTranslator` implementation. Uses Copilot SDK client with a system prompt that instructs translation while preserving markdown/code. Checks Copilot CLI version compatibility (≥ 0.0.411).
- **`internal/subtitle/`** — Manages translation marker blocks in markdown. Uses `<!-- subtitle:LANG:start sha256:HASH -->` / `<!-- subtitle:LANG:end -->` markers for body translations and `<!-- subtitle-title-original:BASE64 -->` / `<!-- subtitle-title:LANG sha256:HASH -->` markers for title translation state. SHA256 hash (first 8 hex chars) of stripped original text enables idempotent updates.
- **`version/`** — Version constant.

## Key Design Decisions

- **Idempotency**: SHA256 hash of original content (after stripping existing markers) prevents redundant translations. `NeedsTranslation()` checks hash before translating.
- **Multi-language coexistence**: Each language gets its own marker block, allowing multiple translations on the same content.
- **Bot filtering**: Bot comments are skipped by default (`--include-bots` to override).
- **Same-language skip**: Translation is skipped when LLM-detected source language (`from`) matches target (`to`). An empty marker block is recorded to avoid LLM calls on subsequent runs.
- **Non-editable content**: 422 Validation Failed errors (e.g. Copilot review comments) are skipped gracefully instead of failing.
- **Title translation**: Titles are translated by default (`--skip-title` to disable). Format: `Original / 翻訳1 / 翻訳2` (sorted by language). Title state is tracked via markers in the body. Original title is base64-encoded in the body for accurate restoration on `--clear`. Titles exceeding 256 characters are skipped.

## Authentication

**Copilot mode** (default): Token resolution for both Copilot SDK and `gh` CLI: `GH_TOKEN` → `GITHUB_TOKEN` → `gh auth` logged-in user.

**BYOK mode** (`--byok`): Uses `GH_SUBTITLE_PROVIDER_API_KEY` env var for external provider authentication. Base URL can be set via `--base-url` flag or `GH_SUBTITLE_PROVIDER_BASE_URL` env var. Supported providers: `openai`, `anthropic`, `azure`, `ollama`.
