#!/usr/bin/env bash
# Configure the GitHub side of this repository with Terraform (terraform/), so
# the workflows in .github/workflows work:
#
#   1. Repository settings: auto-merge, delete branches on merge, merge commit
#      message format, no wiki or projects.
#   2. Secret scanning with push protection, Dependabot alerts and Dependabot
#      security updates. Version updates are configured in
#      .github/dependabot.yml.
#   3. Actions: read-only default token, workflows may approve pull requests.
#   4. CodeQL default setup, for the languages the repository has.
#   5. The RELEASE_APP_ID and RELEASE_APP_PRIVATE_KEY secrets of the GitHub App
#      the release-please workflow uses (an app, not a personal access token:
#      commits an app makes through the API are signed).
#   6. The "main" ruleset: pull requests only, no deletion or force pushes, no
#      bypass, not even for admins.
#   7. The "main-checks" ruleset: the "test", "security" and CodeQL checks, code
#      scanning and Copilot review. Admins may bypass it on a pull request
#      only, to merge a blocked PR; they still cannot push directly.
#   8. The "signed-commits" ruleset: signed commits on every branch, no bypass.
#
# The dev branch is deliberately not created: GitHub deletes it when its pull
# request is merged, and release.sh pushes it again when it is needed.
#
# Usage: github-setup.sh [-r OWNER/REPO] [-n] [-y] [-u] [-S] [-R] [-h]
#
#   -r OWNER/REPO  repository to configure (default: the repository of the
#                  current clone, or $REPOSITORY)
#   -n             dry run: show the plan, change nothing
#   -y             apply without asking for confirmation
#   -u             upgrade the Terraform providers within versions.tf, refresh
#                  the lock file and format the configuration
#   -S             do not manage the secrets
#   -R             do not manage the rulesets (for the first run, before the
#                  workflows are on the default branch; running with -R again
#                  once they exist would remove them)
#   -h             show this help
#
# The secrets are read from RELEASE_APP_ID and RELEASE_APP_PRIVATE_KEY (or
# RELEASE_APP_PRIVATE_KEY_FILE=app.pem). The app must be installed on the
# repository with Contents and Pull requests (read and write) and Metadata
# access. Without them the secrets are not managed.
#
# The state is kept per repository, in the Terraform workspace OWNER-REPO
# (terraform/terraform.tfstate.d/). The script can be re-run; it needs
# terraform and an authenticated GitHub CLI (gh) with admin rights on the
# repository.
#
# The rulesets require the "test" and "security" checks of the workflows that
# exist on the default branch. Get .github/workflows onto main (through a pull
# request) before the first run, and make sure no workflow has a path filter:
# a required check that is never reported blocks the pull request forever.
set -euo pipefail

REPOSITORY="${REPOSITORY:-}"
DRY_RUN=0
YES=0
UPGRADE=0
DO_SECRET=1
DO_RULESET=1

usage() {
    sed -n '2,/^set -euo/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -r|--repo)
            [[ $# -ge 2 ]] || { echo "$1 needs OWNER/REPO" >&2; exit 2; }
            REPOSITORY=$2; shift ;;
        --repo=*)       REPOSITORY=${1#--repo=} ;;
        -n|--dry-run)   DRY_RUN=1 ;;
        -y|--yes)       YES=1 ;;
        -u|--upgrade)   UPGRADE=1 ;;
        -S|--no-secrets)  DO_SECRET=0 ;;
        -R|--no-rulesets) DO_RULESET=0 ;;
        -h|--help)      usage; exit 0 ;;
        *)              echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
    shift
done

command -v terraform >/dev/null || { echo "terraform is required" >&2; exit 2; }
command -v gh >/dev/null || { echo "gh (GitHub CLI) is required" >&2; exit 2; }
gh auth status >/dev/null 2>&1 || { echo "gh is not authenticated; run 'gh auth login'" >&2; exit 2; }

# The repository managed by Terraform is the one this clone belongs to.
REPOSITORY="${REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)}" || true
if [[ ! $REPOSITORY =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
    echo "cannot detect the repository; run inside a clone or pass -r OWNER/REPO" >&2
    exit 2
fi
[[ $(gh api "repos/$REPOSITORY" --jq .permissions.admin) == true ]] ||
    { echo "admin permission on $REPOSITORY is required" >&2; exit 2; }

cd "$(dirname "$0")/../terraform"

# The GitHub provider authenticates with GITHUB_TOKEN.
export GITHUB_TOKEN="${GITHUB_TOKEN:-$(gh auth token)}"
export TF_VAR_owner="${REPOSITORY%%/*}"
export TF_VAR_repository="${REPOSITORY#*/}"
# The release-please GitHub App secrets, taken from the environment:
#   RELEASE_APP_ID           the ID of the app
#   RELEASE_APP_PRIVATE_KEY  the private key, or RELEASE_APP_PRIVATE_KEY_FILE=app.pem
# When not set, Terraform does not manage the secrets.
if [[ -z "${TF_VAR_release_app_id:-}" && -n "${RELEASE_APP_ID:-}" ]]; then
    export TF_VAR_release_app_id="$RELEASE_APP_ID"
fi
if [[ -z "${TF_VAR_release_app_private_key:-}" ]]; then
    if [[ -n "${RELEASE_APP_PRIVATE_KEY:-}" ]]; then
        export TF_VAR_release_app_private_key="$RELEASE_APP_PRIVATE_KEY"
    elif [[ -n "${RELEASE_APP_PRIVATE_KEY_FILE:-}" ]]; then
        [[ -r $RELEASE_APP_PRIVATE_KEY_FILE ]] || { echo "cannot read $RELEASE_APP_PRIVATE_KEY_FILE" >&2; exit 2; }
        TF_VAR_release_app_private_key="$(cat "$RELEASE_APP_PRIVATE_KEY_FILE")"
        export TF_VAR_release_app_private_key
    fi
fi
if [[ -z "${TF_VAR_release_app_id:-}" || -z "${TF_VAR_release_app_private_key:-}" ]]; then
    echo "==> RELEASE_APP_ID / RELEASE_APP_PRIVATE_KEY(_FILE) not both set: release secrets are not managed"
fi

# Secret scanning and CodeQL are not available on private repositories without
# GitHub Advanced Security; override with TF_VAR_advanced_security=true if you
# have it.
if [[ -z "${TF_VAR_advanced_security:-}" ]]; then
    private=$(gh repo view "$REPOSITORY" --json isPrivate --jq .isPrivate 2>/dev/null || echo false)
    if [[ $private == true ]]; then
        export TF_VAR_advanced_security=false
        echo "==> private repository: secret scanning and CodeQL are not managed"
    fi
fi
# The "test" and "security" checks come from tests.yml and security.yml; a check
# whose workflow is missing on the default branch would never report and block
# every pull request. Override with TF_VAR_workflow_checks='["test"]'.
if [[ -z "${TF_VAR_workflow_checks:-}" ]]; then
    checks=()
    for entry in test:tests.yml security:security.yml; do
        if gh api "repos/$REPOSITORY/contents/.github/workflows/${entry#*:}" >/dev/null 2>&1; then
            checks+=("${entry%%:*}")
        else
            echo "==> no ${entry#*:} in $REPOSITORY: the \"${entry%%:*}\" check is not required"
        fi
    done
    TF_VAR_workflow_checks=$(printf '%s\n' ${checks[@]+"${checks[@]}"} | awk 'NF { printf "%s\"%s\"", sep, $0; sep = "," } BEGIN { printf "[" } END { printf "]" }')
    export TF_VAR_workflow_checks
fi

# CodeQL needs code in each language it analyses (the API answers 422
# otherwise) and each language adds a required check that must report, so the
# languages are taken from what GitHub detects. Override with
# TF_VAR_codeql_languages='["go","python"]' (or '[]' to turn CodeQL off).
if [[ -z "${TF_VAR_codeql_languages:-}" ]]; then
    languages=$(gh api "repos/$REPOSITORY/languages" --jq 'keys[]' 2>/dev/null || true)
    codeql=()
    while IFS= read -r language; do
        case $language in
            Go)                      codeql+=(go) ;;
            Python)                  codeql+=(python) ;;
            JavaScript|TypeScript)   codeql+=(javascript-typescript) ;;
            Java|Kotlin)             codeql+=(java-kotlin) ;;
            C|C++)                   codeql+=(c-cpp) ;;
            "C#")                    codeql+=(csharp) ;;
            Ruby)                    codeql+=(ruby) ;;
            Swift)                   codeql+=(swift) ;;
            Rust)                    codeql+=(rust) ;;
        esac
    done <<<"$languages"
    # Drop duplicates (JavaScript and TypeScript, C and C++, ...).
    TF_VAR_codeql_languages=$(printf '%s\n' ${codeql[@]+"${codeql[@]}"} | sort -u | awk 'NF { printf "%s\"%s\"", sep, $0; sep = "," } BEGIN { printf "[" } END { printf "]" }')
    export TF_VAR_codeql_languages
    echo "==> CodeQL languages: $TF_VAR_codeql_languages"
fi


if [[ $DO_SECRET -eq 0 ]]; then
    unset TF_VAR_release_app_id TF_VAR_release_app_private_key
    echo "==> secrets are not managed (-S)"
fi
secrets_managed=1
if [[ -z "${TF_VAR_release_app_id:-}" || -z "${TF_VAR_release_app_private_key:-}" ]]; then
    secrets_managed=0
fi
if [[ $DO_RULESET -eq 0 ]]; then
    export TF_VAR_rulesets=false
    echo "==> rulesets are not managed (-R)"
fi
echo "==> repository ${TF_VAR_owner}/${TF_VAR_repository}"

init_args=(-input=false)
if [[ $UPGRADE -eq 1 ]]; then
    init_args+=(-upgrade)
fi
echo "==> terraform init ${init_args[*]}"
terraform init "${init_args[@]}"

if [[ $UPGRADE -eq 1 ]]; then
    echo "==> terraform providers lock"
    terraform providers lock \
        -platform=darwin_amd64 \
        -platform=darwin_arm64 \
        -platform=linux_amd64 \
        -platform=linux_arm64
    echo "==> terraform fmt"
    terraform fmt -recursive
fi

# One workspace, and so one state file (terraform.tfstate.d/<workspace>/), per
# repository: the state describes a single repository, and sharing it between
# repositories would plan to destroy the previous one.
workspace="${REPOSITORY//\//-}"
if terraform workspace list | sed 's/^[* ]*//' | grep -qxF "$workspace"; then
    terraform workspace select "$workspace"
else
    # A state from before the workspaces sits in the default workspace; adopt
    # it when it belongs to this repository, and start empty otherwise.
    seed=()
    if [[ -s terraform.tfstate ]] && grep -q "\"full_name\": \"$REPOSITORY\"" terraform.tfstate; then
        echo "==> adopting terraform.tfstate for $REPOSITORY"
        seed=(-state=terraform.tfstate)
    fi
    terraform workspace new ${seed[@]+"${seed[@]}"} "$workspace"
fi
echo "==> workspace $workspace"

echo "==> terraform validate"
terraform validate

# Without values for the secrets Terraform would plan to delete the ones it
# created earlier. Forget them instead: they stay on GitHub, untouched.
if [[ $secrets_managed -eq 0 ]]; then
    for address in 'github_actions_secret.release_app_id[0]' 'github_actions_secret.release_app_private_key[0]'; do
        if terraform state list 2>/dev/null | grep -qxF "$address"; then
            if [[ $DRY_RUN -eq 1 ]]; then
                echo "==> $address would be dropped from the state, not deleted on GitHub (the plan below shows it as destroyed)"
            else
                echo "==> dropping $address from the state; the secret stays on GitHub"
                terraform state rm "$address" >/dev/null
            fi
        fi
    done
fi

# The plan file can hold secret values: keep it in a private temp directory.
planfile="$(mktemp -d)/plan"
trap 'rm -rf "$(dirname "$planfile")"' EXIT

echo "==> terraform plan"
status=0
# Terraform's own summary after a saved plan only repeats the apply command,
# so print the plan with "show" instead.
terraform plan -input=false -detailed-exitcode -out="$planfile" >"$planfile.log" 2>&1 || status=$?
if [[ $status -eq 1 ]]; then
    cat "$planfile.log" >&2
    exit 1
fi
if [[ $status -eq 0 ]]; then
    echo "Nothing to change: $REPOSITORY is up to date."
    exit 0
fi
terraform show "$planfile"
if [[ $DRY_RUN -eq 1 ]]; then
    exit 0
fi

if [[ $YES -eq 0 ]]; then
    read -r -p "Apply these changes to $REPOSITORY? [y/N] " answer
    [[ $answer == y || $answer == Y || $answer == yes ]] || { echo "Aborted."; exit 1; }
fi
echo "==> terraform apply"
terraform apply -input=false "$planfile"

echo "Done."
if [[ $DO_RULESET -eq 1 ]]; then
    echo "main now requires pull requests and signed commits; push changes through a PR."
fi
if [[ $UPGRADE -eq 1 ]]; then
    echo "Review changes to terraform/.terraform.lock.hcl and commit them."
fi
