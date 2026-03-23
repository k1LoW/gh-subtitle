# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gh-subtitle is a GitHub CLI (`gh`) extension that translates PR/Issue/Discussion bodies and comments, appending translations as "subtitles" using idempotent HTML comment markers. It uses the GitHub Copilot SDK for translation.

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

- **`cmd/root.go`** — CLI entry point and orchestration. Dispatches to `runTranslate()` or `runClear()`. Lazily initializes translator, iterates over target languages, batches items for translation.
- **`internal/github/`** — GitHub API interactions via `gh` CLI commands. Uses REST API for PRs/Issues, GraphQL for Discussions. `ParseURL()` determines content type from URL.
- **`internal/translator/`** — `Translator` interface with `CopilotTranslator` implementation. Uses Copilot SDK client with a system prompt that instructs translation while preserving markdown/code. Checks Copilot CLI version compatibility (≥ 0.0.411).
- **`internal/subtitle/`** — Manages translation marker blocks in markdown. Uses `<!-- subtitle:LANG:start sha256:HASH -->` / `<!-- subtitle:LANG:end -->` markers. SHA256 hash (first 8 hex chars) of stripped original text enables idempotent updates.
- **`version/`** — Version constant.

## Key Design Decisions

- **Idempotency**: SHA256 hash of original content (after stripping existing markers) prevents redundant translations. `NeedsTranslation()` checks hash before translating.
- **Multi-language coexistence**: Each language gets its own marker block, allowing multiple translations on the same content.
- **Bot filtering**: Bot comments are skipped by default (`--include-bots` to override).
- **Same-language skip**: Translation is skipped when LLM-detected source language (`from`) matches target (`to`). An empty marker block is recorded to avoid LLM calls on subsequent runs.
- **Non-editable content**: 422 Validation Failed errors (e.g. Copilot review comments) are skipped gracefully instead of failing.

## Authentication

**Copilot mode** (default): Token resolution for Copilot SDK: `GITHUB_TOKEN` → `gh auth` logged-in user. The `gh` CLI uses its own token resolution for API calls.

**BYOK mode** (`--byok`): Uses `GH_SUBTITLE_PROVIDER_API_KEY` env var for external provider authentication. Base URL can be set via `--base-url` flag or `GH_SUBTITLE_PROVIDER_BASE_URL` env var. Supported providers: `openai`, `anthropic`, `azure`, `ollama`.
