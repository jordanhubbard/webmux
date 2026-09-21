#!/usr/bin/env bash
# Release helper for WebMux — PR-based workflow.
#
# Usage:
#   ./scripts/release.sh prepare [major|minor|patch]
#       Bumps versions, updates the changelog, runs tests, and opens a
#       release pull request. Does not touch main directly.
#
#   ./scripts/release.sh publish
#       Run after the release PR has been reviewed and merged into main.
#       Validates that package/lockfile versions agree, tags the merge
#       commit, pushes the tag, and creates the GitHub release.
#
# Batch mode (non-interactive): BATCH=yes ./scripts/release.sh prepare [bump]

set -euo pipefail

BATCH_MODE="${BATCH:-no}"

# ── Colors ────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { echo -e "${BLUE}▸  $1${NC}"; }
success() { echo -e "${GREEN}✓  $1${NC}"; }
warn()    { echo -e "${YELLOW}!  $1${NC}"; }
error()   { echo -e "${RED}✗  $1${NC}"; exit 1; }

# ── Prerequisites ─────────────────────────────────────────────────────
check_gh() {
    if ! command -v gh &>/dev/null; then
        error "GitHub CLI (gh) is not installed. Install with: brew install gh"
    fi
    if ! gh auth status --active &>/dev/null; then
        error "GitHub CLI is not authenticated. Run: gh auth login"
    fi
}

check_clean_worktree() {
    if [[ -n $(git status --porcelain) ]]; then
        error "Working directory is not clean. Commit or stash changes first."
    fi
}

check_on_main_up_to_date() {
    local branch
    branch=$(git rev-parse --abbrev-ref HEAD)
    if [[ "$branch" != "main" ]]; then
        error "Not on main branch (currently on: $branch). Switch to main first."
    fi
    git fetch origin main --quiet
    local local_head remote_head
    local_head=$(git rev-parse HEAD)
    remote_head=$(git rev-parse origin/main)
    if [[ "$local_head" != "$remote_head" ]]; then
        error "Local main is not up to date with origin/main. Run: git pull --ff-only origin main"
    fi
}

# ── Version helpers ───────────────────────────────────────────────────
get_current_version() {
    git tag -l 'v*' | sort -V | tail -1 | sed 's/^v//'
}

get_package_version() {
    node scripts/release-metadata.mts read "$1"
}

calculate_next_version() {
    local current=$1 bump_type=$2
    IFS='.' read -r major minor patch <<< "$current"
    case $bump_type in
        major) major=$((major + 1)); minor=0; patch=0 ;;
        minor) minor=$((minor + 1)); patch=0 ;;
        patch) patch=$((patch + 1)) ;;
        *)     error "Invalid bump type: $bump_type (use major, minor, or patch)" ;;
    esac
    echo "$major.$minor.$patch"
}

bump_package_versions() {
    local version=$1
    info "Bumping package.json versions to $version..."
    node scripts/release-metadata.mts bump "$version"
    success "package.json files updated to $version"
}

# Ensures root/frontend package.json versions and the root lockfile's
# per-workspace version entries all agree before a release is tagged.
validate_version_consistency() {
    node scripts/release-metadata.mts validate
}

# ── Changelog ─────────────────────────────────────────────────────────
generate_changelog_entry() {
    local prev_version=$1 new_version=$2
    local date; date=$(date +%Y-%m-%d)

    info "Generating changelog from v$prev_version to HEAD..." >&2

    local commits
    if git rev-parse "v$prev_version" &>/dev/null; then
        commits=$(git log "v$prev_version"..HEAD --pretty=format:"%h %s" --no-merges)
    else
        commits=$(git log --pretty=format:"%h %s" --no-merges)
    fi

    local added="" changed="" fixed="" other=""
    while IFS= read -r line; do
        if [[ $line =~ ^[a-f0-9]+\ feat(\(.*\))?:\ (.*) ]];     then added+="- ${BASH_REMATCH[2]}\n"
        elif [[ $line =~ ^[a-f0-9]+\ fix(\(.*\))?:\ (.*) ]];    then fixed+="- ${BASH_REMATCH[2]}\n"
        elif [[ $line =~ ^[a-f0-9]+\ refactor(\(.*\))?:\ (.*) ]]; then changed+="- ${BASH_REMATCH[2]}\n"
        else
            local msg; msg=$(echo "$line" | cut -d' ' -f2-)
            other+="- $msg\n"
        fi
    done <<< "$commits"

    local entry="## [$new_version] - $date\n\n"
    [[ -n "$added"   ]] && entry+="### Added\n$added\n"
    [[ -n "$changed" ]] && entry+="### Changed\n$changed\n"
    [[ -n "$fixed"   ]] && entry+="### Fixed\n$fixed\n"
    [[ -n "$other"   ]] && entry+="### Other\n$other\n"

    echo -e "$entry"
}

update_changelog() {
    local changelog_entry=$1
    local changelog_file="CHANGELOG.md"

    info "Updating $changelog_file..."

    if [[ ! -f "$changelog_file" ]]; then
        error "CHANGELOG.md not found. Run 'make changelog-init' first."
    fi

    local temp_file entry_file
    temp_file=$(mktemp)
    entry_file=$(mktemp)
    echo -e "$changelog_entry" > "$entry_file"

    awk '
        /^## \[Unreleased\]/ {
            print $0
            print ""
            while ((getline line < "'"$entry_file"'") > 0) print line
            close("'"$entry_file"'")
            next
        }
        { print }
    ' "$changelog_file" > "$temp_file"

    mv "$temp_file" "$changelog_file"
    rm -f "$entry_file"
    success "CHANGELOG.md updated"
}

# ── prepare: create the release branch + PR ─────────────────────────────
cmd_prepare() {
    check_gh
    check_clean_worktree
    check_on_main_up_to_date

    local current_version
    current_version=$(get_current_version)
    if [[ -z "$current_version" ]]; then
        current_version="0.0.0"
        info "No prior tags found — this will be the first release"
    else
        info "Current version: v$current_version"
    fi

    local bump_type="${1:-patch}"
    [[ "$bump_type" =~ ^(major|minor|patch)$ ]] || error "Invalid argument: $bump_type (use major, minor, or patch)"

    local next_version
    next_version=$(calculate_next_version "$current_version" "$bump_type")

    printf '\n'
    printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
    printf '  Current: v%s\n' "$current_version"
    printf '  Next:    v%s (%s)\n' "$next_version" "$bump_type"
    printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
    printf '\n'

    if [[ "$BATCH_MODE" == "yes" ]]; then
        info "Batch mode: proceeding with release v$next_version"
    else
        read -rp "Proceed with release v$next_version? (y/n) " -n 1
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || { warn "Release cancelled"; exit 0; }
    fi

    local changelog_entry
    changelog_entry=$(generate_changelog_entry "$current_version" "$next_version")

    printf '\n'
    info "Generated changelog entry:"
    printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
    echo -e "$changelog_entry"
    printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
    printf '\n'

    if [[ "$BATCH_MODE" != "yes" ]]; then
        read -rp "Does this look correct? (y/n) " -n 1
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            warn "Edit CHANGELOG.md manually and re-run."
            exit 0
        fi
    fi

    local release_branch="release/v$next_version"
    info "Creating release branch $release_branch..."
    git checkout -b "$release_branch"

    update_changelog "$changelog_entry"
    bump_package_versions "$next_version"
    validate_version_consistency > /dev/null

    info "Running tests..."
    local test_output; test_output=$(mktemp)
    if ! make test > "$test_output" 2>&1; then
        cat "$test_output"
        rm -f "$test_output"
        git checkout main
        git branch -D "$release_branch"
        error "Tests failed. Fix before releasing."
    fi
    local test_status
    test_status=$(grep -E "Tests:|passed|failed" "$test_output" | tail -1 || echo "All tests passed")
    rm -f "$test_output"
    success "Tests passed"

    info "Committing release changes..."
    git add CHANGELOG.md webmux/frontend/package.json webmux/package.json webmux/package-lock.json
    git commit -m "Release v$next_version"

    info "Pushing $release_branch..."
    git push -u origin "$release_branch"

    local commit_count release_notes
    if git rev-parse "v$current_version" &>/dev/null; then
        commit_count=$(git rev-list --count "v$current_version"..HEAD)
        release_notes=$(git log "v$current_version"..HEAD --pretty=format:"- %s" --no-merges)
    else
        commit_count=$(git rev-list --count HEAD)
        release_notes=$(git log --pretty=format:"- %s" --no-merges)
    fi

    local body_file; body_file=$(mktemp)
    cat > "$body_file" << EOF
## Release v$next_version

### Statistics
- **Commits since v$current_version**: $commit_count
- **Test status**: $test_status

### Changes

$release_notes

---
Merge this PR, then run \`./scripts/release.sh publish\` from an up-to-date \`main\` to tag and publish the GitHub release.
EOF

    info "Opening release pull request..."
    local pr_url
    pr_url=$(gh pr create --title "Release v$next_version" --body-file "$body_file" --base main --head "$release_branch")
    rm -f "$body_file"

    git checkout main

    printf '\n'
    printf '╔══════════════════════════════════════╗\n'
    printf '║      Release PR opened               ║\n'
    printf '╚══════════════════════════════════════╝\n'
    printf '\n'
    printf 'Pull request: %s\n' "$pr_url"
    printf 'After it is reviewed and merged, run: ./scripts/release.sh publish\n\n'
}

# ── publish: tag + GitHub release after the PR is merged ────────────────
cmd_publish() {
    check_gh
    check_clean_worktree
    check_on_main_up_to_date

    info "Validating package/lockfile version consistency..."
    local version
    version=$(validate_version_consistency)
    success "Versions agree: v$version"

    if git rev-parse "v$version" &>/dev/null; then
        error "Tag v$version already exists. Nothing to publish."
    fi

    local prev_version
    prev_version=$(get_current_version)
    [[ -z "$prev_version" ]] && prev_version="0.0.0"

    if [[ "$prev_version" == "$version" ]]; then
        error "package.json version (v$version) matches the latest tag. Did you forget to merge the release PR?"
    fi

    local commit_count release_notes
    if git rev-parse "v$prev_version" &>/dev/null; then
        commit_count=$(git rev-list --count "v$prev_version"..HEAD)
        release_notes=$(git log "v$prev_version"..HEAD --pretty=format:"- %s" --no-merges)
    else
        commit_count=$(git rev-list --count HEAD)
        release_notes=$(git log --pretty=format:"- %s" --no-merges)
    fi

    local repo_url
    repo_url=$(gh repo view --json url -q .url 2>/dev/null || echo "")
    local compare_url="${repo_url}/compare/v${prev_version}...v${version}"

    local notes_file; notes_file=$(mktemp)
    cat > "$notes_file" << EOF
## WebMux v$version

### Statistics
- **Commits since v$prev_version**: $commit_count

### Changes

$release_notes

### Links
- [Full changelog]($compare_url)
- [Documentation](${repo_url}/tree/main/docs)
EOF

    if [[ "$BATCH_MODE" != "yes" ]]; then
        read -rp "Tag and publish v$version? (y/n) " -n 1
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || { warn "Publish cancelled"; rm -f "$notes_file"; exit 0; }
    fi

    info "Creating git tag v$version..."
    git tag -a "v$version" -m "Release v$version"

    info "Pushing tag..."
    git push origin "v$version"

    info "Creating GitHub release..."
    gh release create "v$version" --title "v$version" --notes-file "$notes_file"
    rm -f "$notes_file"

    printf '\n'
    printf '╔══════════════════════════════════════╗\n'
    printf '║         Release Published!           ║\n'
    printf '╚══════════════════════════════════════╝\n'
    printf '\n'
    printf 'Release: %s/releases/tag/v%s\n\n' "$repo_url" "$version"
}

# ── Main ──────────────────────────────────────────────────────────────
usage() {
    cat <<EOF
Usage:
  $0 prepare [major|minor|patch]   Bump versions, update changelog, open a release PR
  $0 publish                       Tag and publish after the release PR is merged

Batch mode (non-interactive): BATCH=yes $0 prepare [bump]
EOF
    exit 1
}

main() {
    local subcommand="${1:-}"
    [[ $# -gt 0 ]] && shift || true

    printf '\n'
    printf '╔══════════════════════════════════════╗\n'
    printf '║    WebMux Release Helper             ║\n'
    printf '╚══════════════════════════════════════╝\n'
    printf '\n'

    [[ "$BATCH_MODE" == "yes" ]] && info "Running in BATCH mode (non-interactive)"

    case "$subcommand" in
        prepare) cmd_prepare "$@" ;;
        publish) cmd_publish "$@" ;;
        *) usage ;;
    esac
}

main "$@"
