---
name: codex-switcher-release
description: Use when asked to cut a new codex-switcher release version (bump VERSION, run tests/build, commit, tag, push, create GitHub release, and upload release artifacts).
---

# Codex Switcher Release

## Overview
Execute a deterministic release workflow for this repository. Keep release steps consistent and avoid accidentally committing local artifacts.

## Preconditions

- Ensure current directory is repository root.
- Ensure GitHub CLI authentication is available (`gh auth status`).
- Ensure working tree is clean except explicitly ignored local files (for example screenshots like `img_*.png`).
- Refuse to continue if there are unexpected unstaged/staged code changes unrelated to release.

## Inputs

- `version`: release version tag in form `vX.Y.Z`.
- `title`: optional release title (default: `codex-switcher <version>`).
- `notes`: optional release notes body.

## Release Workflow

1. Update `VERSION` exactly to the requested tag string.
2. Run checks and build artifacts:
   - `go test ./...`
   - `./build-release.sh`
3. Verify built binary version matches `VERSION`:
   - `./dist/releases/codex-switcher-linux-x86_64 --version`
4. Review git state and stage only release-relevant files:
   - include `VERSION` and code/doc changes that belong to this release
   - never stage screenshots or ad-hoc local files
5. Create release commit:
   - preferred message: `Release <version>`
6. Create annotated tag:
   - `git tag -a <version> -m "Release <version>"`
7. Push commit and tag:
   - `git push origin main`
   - `git push origin <version>`
8. Create GitHub release and upload all artifacts from `dist/releases/`:
   - `codex-switcher-linux-x86_64`
   - `codex-switcher-windows-x86_64.exe`
   - `codex-switcher-macos-x86_64`
   - `codex-switcher-macos-arm64`
9. Verify release and assets:
   - `gh release view <version> --json url,assets`

## Command Templates

```bash
go test ./... && ./build-release.sh
```

```bash
git add VERSION <other-release-files>
git commit -m "Release vX.Y.Z"
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin main
git push origin vX.Y.Z
```

```bash
gh release create vX.Y.Z \
  "dist/releases/codex-switcher-linux-x86_64" \
  "dist/releases/codex-switcher-windows-x86_64.exe" \
  "dist/releases/codex-switcher-macos-x86_64" \
  "dist/releases/codex-switcher-macos-arm64" \
  --title "codex-switcher vX.Y.Z" \
  --notes "<release notes>"
```

## Guardrails

- Never commit files under `dist/` (artifacts are uploaded to release, not stored in git).
- Never include `img_*.png` or other local debug files in release commits.
- Never force-push tags or branches.
- If tag already exists, stop and require explicit operator decision.

## Expected Output

- Print commit SHA, tag name, and release URL.
- Confirm all four artifacts are present on the release.
