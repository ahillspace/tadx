#!/bin/sh

set -eu

test_root=$(mktemp -d "${TMPDIR:-/tmp}/tadx-installer-test.XXXXXXXX")
cleanup() {
    rm -rf "$test_root"
}
trap cleanup EXIT HUP INT TERM

CDPATH=''
export CDPATH
repository_root=$(cd -- "$(dirname "$0")/../.." && pwd)
release_directory="${test_root}/releases"
fake_bin="${test_root}/fake-bin"
home_directory="${test_root}/home"
install_directory="${home_directory}/bin space's"
mkdir -p "$release_directory" "$fake_bin" "$home_directory"

printf '%s\n' '#!/bin/sh' 'if [ "${1:-}" = agent ]; then printf "%s\n" "$*" >> "$TADX_TEST_GUIDANCE_LOG"; [ "${TADX_TEST_GUIDANCE_FAIL:-0}" = 0 ]; elif [ "${1:-}" = completion ]; then printf "%s\n" "# test completion"; else printf "%s\n" "tadx test 1.2.3"; fi' > "${test_root}/tadx"
chmod 0755 "${test_root}/tadx"
tar -czf "${release_directory}/tadx_1.2.3_linux_amd64.tar.gz" -C "$test_root" tadx
if command -v sha256sum >/dev/null 2>&1; then
    release_hash=$(sha256sum "${release_directory}/tadx_1.2.3_linux_amd64.tar.gz" | awk '{ print $1 }')
else
    release_hash=$(shasum -a 256 "${release_directory}/tadx_1.2.3_linux_amd64.tar.gz" | awk '{ print $1 }')
fi
printf '%s  %s\n' "$release_hash" 'tadx_1.2.3_linux_amd64.tar.gz' > "${release_directory}/checksums.txt"

# The single-quoted lines are the literal source of the fake curl command.
# shellcheck disable=SC2016
printf '%s\n' \
    '#!/bin/sh' \
    'set -eu' \
    'destination=' \
    'url=' \
    'while [ "$#" -gt 0 ]; do' \
    '  case "$1" in' \
    '    -o) shift; destination=$1 ;;' \
    '    https://*) url=$1 ;;' \
    '  esac' \
    '  shift' \
    'done' \
    'printf "%s\n" "$url" >> "${TADX_TEST_CURL_LOG}"' \
    'cp "${TADX_TEST_RELEASES}/$(basename "$url")" "$destination"' > "${fake_bin}/curl"
chmod 0755 "${fake_bin}/curl"

# The single-quoted lines are the literal source of the fake gh command.
# shellcheck disable=SC2016
printf '%s\n' \
    '#!/bin/sh' \
    'set -eu' \
    'printf "%s\n" "$*" >> "${TADX_TEST_GH_LOG}"' \
    'if [ "${1:-}" = auth ]; then' \
    '  [ "${TADX_TEST_GH_AUTH:-success}" = success ]' \
    '  exit' \
    'fi' \
    'pattern=' \
    'destination=' \
    'while [ "$#" -gt 0 ]; do' \
    '  case "$1" in' \
    '    --pattern) shift; pattern=$1 ;;' \
    '    --output) shift; destination=$1 ;;' \
    '  esac' \
    '  shift' \
    'done' \
    'cp "${TADX_TEST_RELEASES}/${pattern}" "$destination"' > "${fake_bin}/gh"
chmod 0755 "${fake_bin}/gh"

# The single-quoted lines are the literal source of the fake uname command.
# shellcheck disable=SC2016
printf '%s\n' \
    '#!/bin/sh' \
    'case "${1:-}" in' \
    '  -s) printf "%s\n" Linux ;;' \
    '  -m) printf "%s\n" x86_64 ;;' \
    '  *) printf "%s\n" Linux ;;' \
    'esac' > "${fake_bin}/uname"
chmod 0755 "${fake_bin}/uname"

export HOME="$home_directory"
export SHELL='/bin/sh'
export TADX_TEST_RELEASES="$release_directory"
export TADX_TEST_GH_LOG="${test_root}/gh.log"
export TADX_TEST_CURL_LOG="${test_root}/curl.log"
export TADX_TEST_GUIDANCE_LOG="${test_root}/guidance.log"
PATH="${fake_bin}:${PATH}"
export PATH

sh "${repository_root}/scripts/install.sh" install --version latest --install-dir "$install_directory" >/dev/null
[ -x "${install_directory}/tadx" ]
grep -Fq 'agent install --target auto' "$TADX_TEST_GUIDANCE_LOG"
export TADX_TEST_GUIDANCE_FAIL=1
if sh "${repository_root}/scripts/install.sh" --version 1.2.3 --install-dir "$install_directory" --no-modify-path --no-completion >/dev/null 2>&1; then
    printf '%s\n' 'Guidance failure incorrectly succeeded.' >&2
    exit 1
fi
unset TADX_TEST_GUIDANCE_FAIL
[ "$("${install_directory}/tadx")" = 'tadx test 1.2.3' ]
[ "$("${install_directory}/tadx")" = 'tadx test 1.2.3' ]
[ "$(grep -c '# tadx-installer-path' "${home_directory}/.profile")" -eq 1 ]
grep -Fq 'auth status --hostname github.com' "$TADX_TEST_GH_LOG"
grep -Fq 'release download' "$TADX_TEST_GH_LOG"
[ ! -e "$TADX_TEST_CURL_LOG" ]

export TADX_TEST_GH_AUTH='fail'
PATH="${install_directory}:${PATH}" sh "${repository_root}/scripts/install.sh" install --version 1.2.3 --install-dir "$install_directory" >/dev/null
[ "$(grep -c '# tadx-installer-path' "${home_directory}/.profile")" -eq 1 ]
[ -s "$TADX_TEST_CURL_LOG" ]

mkdir -p "${home_directory}/.config/tadx" "${home_directory}/.codex/skills/tadx"
printf '%s\n' preserved > "${home_directory}/.config/tadx/config.yaml"
printf '%s\n' preserved > "${home_directory}/.codex/skills/tadx/SKILL.md"
sh "${repository_root}/scripts/install.sh" uninstall --install-dir "$install_directory" >/dev/null
[ ! -e "${install_directory}/tadx" ]
[ "$(grep -c '# tadx-installer-path' "${home_directory}/.profile" || true)" -eq 0 ]
[ -f "${home_directory}/.config/tadx/config.yaml" ]
[ -f "${home_directory}/.codex/skills/tadx/SKILL.md" ]

if sh "${repository_root}/scripts/install.sh" install --version '../unsafe' --install-dir "$install_directory" >/dev/null 2>&1; then
    printf '%s\n' 'Expected an unsafe version to fail.' >&2
    exit 1
fi

for completion_shell in bash zsh fish; do
    export SHELL="/bin/$completion_shell"
    case "$completion_shell" in
        bash) completion_profile="${home_directory}/.bashrc" ;;
        zsh) export ZDOTDIR="${home_directory}/custom-zsh"; completion_profile="${ZDOTDIR}/.zshrc" ;;
        fish) export XDG_CONFIG_HOME="${home_directory}/custom-config"; completion_profile="${XDG_CONFIG_HOME}/fish/config.fish" ;;
    esac
    mkdir -p "$(dirname "$completion_profile")"
    printf '%s\n' '# user configuration' > "$completion_profile"
    sh "${repository_root}/scripts/install.sh" --install-dir "$install_directory" >/dev/null
    grep -Fq "completion $completion_shell" "$completion_profile"
    [ "$(grep -c '# tadx-installer-completion' "$completion_profile")" -eq 1 ]
    [ "$(cat "${completion_profile}.tadx-backup")" = '# user configuration' ]
    cp "$completion_profile" "${test_root}/first-profile"
    sh "${repository_root}/scripts/install.sh" --install-dir "$install_directory" >/dev/null
    cmp "$completion_profile" "${test_root}/first-profile"
    if [ "$completion_shell" = bash ]; then
        bash -n "$completion_profile"
        bash -c '. "$1"' bash "$completion_profile"
    fi
    sh "${repository_root}/scripts/install.sh" uninstall --install-dir "$install_directory" >/dev/null
    [ "$(cat "$completion_profile")" = '# user configuration' ]
    sh "${repository_root}/scripts/install.sh" --install-dir "$install_directory" --no-modify-path --no-completion >/dev/null
    [ "$(cat "$completion_profile")" = '# user configuration' ]
done

printf '%s\n' 'install.sh tests passed'
