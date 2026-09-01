# Agent Instructions

Public work enters through GitHub Issues and pull requests. Maintainers use [mac](https://github.com/jordanhubbard/mac) for internal task tracking and work dispatch. This repository does not use beads.

## Quick Reference

```bash
gh issue list                         # Find available work
gh issue view <number>                # View issue details
gh issue edit <number> --add-assignee @me
gh pr create --fill                   # Open a pull request
gh pr checks                          # Verify CI
```

## GitHub Workflow

- Track planned work and follow-ups in GitHub Issues. Create an issue before substantial code changes when one does not already exist.
- Work on a branch and reference the issue in commits and the pull request body.
- Use pull requests for code and documentation changes. Do not push feature work directly to `main`.
- Run relevant tests, linters, and builds before opening a pull request.
- Merge only after required checks pass. Use `Closes #<number>` in the pull request body when merging should close an issue.
- Releases are cut from a clean, up-to-date `main` branch after the release changes have been reviewed and merged.

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work through the PR or update open issues
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
