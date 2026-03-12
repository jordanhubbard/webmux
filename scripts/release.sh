#!/usr/bin/env bash
# Automated release script for WebMux
# Usage: ./scripts/release.sh [major|minor|patch]
# Batch mode: BATCH=yes ./scripts/release.sh [major|minor|patch]

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
check_prerequisites() {
    info "Checking prerequisites..."

    if ! command -v gh &>/dev/null; then
        error "GitHub CLI (gh) is not installed. Install with: brew install gh"
    fi

    if ! gh auth status &>/dev/null; then
        error "GitHub CLI is not authenticated. Run: gh auth login"
    fi

    if [[ -n $(git status --porcelain) ]]; then
        error "Working directory is not clean. Commit or stash changes first."
    fi

    local branch
    branch=$(git rev-parse --abbrev-ref HEAD)
    if [[ "$branch" != "main" ]]; then
        warn "Not on main branch (currently on: $branch)"
        if [[ "$BATCH_MODE" == "yes" ]]; then
            error "Not on main branch in batch mode. Switch to main first."
        fi
        read -rp "Continue anyway? (y/n) " -n 1
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || error "Aborted by user"
    fi

    success "Prerequisites check passed"
}

# ── Version helpers ───────────────────────────────────────────────────
get_current_version() {
    git tag -l 'v*' | sort -V | tail -1 | sed 's/^v//'
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
    sed -i.bak "s/\"version\": \"[^\"]*\"/\"version\": \"$version\"/" webmux/frontend/package.json && rm -f webmux/frontend/package.json.bak
    sed -i.bak "s/\"version\": \"[^\"]*\"/\"version\": \"$version\"/" webmux/backend/package.json  && rm -f webmux/backend/package.json.bak
    success "package.json files updated to $version"
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

# ── Create release ────────────────────────────────────────────────────
create_release() {
    local version=$1 prev_version=$2 test_status=$3

    info "Creating release v$version..."

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
- **Test status**: $test_status

### Changes

$release_notes

### Links
- [Full changelog]($compare_url)
- [Documentation](${repo_url}/tree/main/docs)
EOF

    # Commit changelog + version bump
    info "Committing changelog and version bump..."
    git add CHANGELOG.md webmux/frontend/package.json webmux/backend/package.json
    if ! git diff --cached --quiet; then
        git commit -m "Release v$version

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
    fi

    # Tag, push, GitHub release
    info "Creating git tag v$version..."
    git tag -a "v$version" -m "Release v$version"

    info "Pushing to origin..."
    git push origin main
    git push origin "v$version"

    info "Creating GitHub release..."
    gh release create "v$version" --title "v$version" --notes-file "$notes_file"
    rm -f "$notes_file"

    success "Release v$version created!"
}

# ── Main ──────────────────────────────────────────────────────────────
main() {
    printf '\n'
    printf '╔══════════════════════════════════════╗\n'
    printf '║    WebMux Automated Release Script   ║\n'
    printf '╚══════════════════════════════════════╝\n'
    printf '\n'

    [[ "$BATCH_MODE" == "yes" ]] && info "Running in BATCH mode (non-interactive)"

    check_prerequisites

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

    # Changelog
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

    update_changelog "$changelog_entry"
    bump_package_versions "$next_version"

    # Tests
    info "Running tests..."
    local test_output; test_output=$(mktemp)
    if ! make test > "$test_output" 2>&1; then
        cat "$test_output"
        rm -f "$test_output"
        if [[ "$BATCH_MODE" == "yes" ]]; then
            error "Tests failed. Fix before releasing."
        fi
        warn "Tests failed! Continue anyway?"
        read -rp "(y/n) " -n 1; echo
        [[ $REPLY =~ ^[Yy]$ ]] || error "Release cancelled due to test failures"
    fi
    local test_status
    test_status=$(grep -E "Tests:|passed|failed" "$test_output" | tail -1 || echo "All tests passed")
    rm -f "$test_output"
    success "Tests passed"

    create_release "$next_version" "$current_version" "$test_status"

    printf '\n'
    printf '╔══════════════════════════════════════╗\n'
    printf '║         Release Complete!            ║\n'
    printf '╚══════════════════════════════════════╝\n'
    printf '\n'
    printf 'Release: https://github.com/jordanhubbard/webmux/releases/tag/v%s\n\n' "$next_version"
}

main "$@"
