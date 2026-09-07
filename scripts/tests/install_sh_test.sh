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
install_directory="${home_directory}/bin"
mkdir -p "$release_directory" "$fake_bin" "$home_directory"

printf '%s\n' '#!/bin/sh' 'printf "%s\n" "tadx test 1.2.3"' > "${test_root}/tadx"
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
    'cp "${TADX_TEST_RELEASES}/$(basename "$url")" "$destination"' > "${fake_bin}/curl"
chmod 0755 "${fake_bin}/curl"

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
PATH="${fake_bin}:${PATH}"
export PATH

sh "${repository_root}/scripts/install.sh" install --version 1.2.3 --install-dir "$install_directory" >/dev/null
[ -x "${install_directory}/tadx" ]
[ "$("${install_directory}/tadx")" = 'tadx test 1.2.3' ]
[ "$(grep -c '# tadx-installer-path' "${home_directory}/.profile")" -eq 1 ]

PATH="${install_directory}:${PATH}" sh "${repository_root}/scripts/install.sh" install --version 1.2.3 --install-dir "$install_directory" >/dev/null
[ "$(grep -c '# tadx-installer-path' "${home_directory}/.profile")" -eq 1 ]

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

printf '%s\n' 'install.sh tests passed'
