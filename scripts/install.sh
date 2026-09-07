#!/bin/sh

set -eu

repository='ahillspace/tadx'
path_marker='# tadx-installer-path'
action='install'
version=${TADX_VERSION:-latest}
install_dir=${TADX_INSTALL_DIR:-"${HOME}/.local/bin"}
modify_path=1

usage() {
    printf '%s\n' 'Usage: install.sh [install|uninstall] [--version VERSION] [--install-dir DIRECTORY] [--no-modify-path]'
}

fail() {
    printf 'TADX installer error: %s\n' "$1" >&2
    exit 1
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        install|uninstall)
            action=$1
            shift
            ;;
        --version)
            [ "$#" -ge 2 ] || fail '--version requires a value.'
            version=$2
            shift 2
            ;;
        --install-dir)
            [ "$#" -ge 2 ] || fail '--install-dir requires a value.'
            install_dir=$2
            shift 2
            ;;
        --no-modify-path)
            modify_path=0
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            usage >&2
            fail "Unknown argument: $1"
            ;;
    esac
done

case "$install_dir" in
    /*) ;;
    *) install_dir=$(pwd)/$install_dir ;;
esac

profile_path() {
    case "${SHELL:-}" in
        */zsh) printf '%s\n' "${HOME}/.zshrc" ;;
        */bash) printf '%s\n' "${HOME}/.bashrc" ;;
        *) printf '%s\n' "${HOME}/.profile" ;;
    esac
}

remove_managed_path() {
    profile=$(profile_path)
    [ -f "$profile" ] || return 0
    temporary_profile="${profile}.tadx.$$"
    cp -p "$profile" "$temporary_profile"
    awk -v marker="$path_marker" 'index($0, marker) == 0' "$profile" > "$temporary_profile"
    mv -f "$temporary_profile" "$profile"
}

add_managed_path() {
    profile=$(profile_path)
    if ! { [ -f "$profile" ] && grep -Fq "$path_marker" "$profile"; }; then
        case ":${PATH}:" in
            *":${install_dir}:"*) return 0 ;;
        esac
    fi

    if [ ! -e "$profile" ]; then
        umask 077
        : > "$profile"
    fi
    remove_managed_path

    escaped_dir=$(printf '%s' "$install_dir" | sed "s/'/'\\\\''/g")
    printf "\nexport PATH='%s':\"\$PATH\" %s\n" "$escaped_dir" "$path_marker" >> "$profile"
}

if [ "$action" = 'uninstall' ]; then
    rm -f "${install_dir}/tadx"
    remove_managed_path
    if [ -d "$install_dir" ] && [ -z "$(ls -A "$install_dir")" ]; then
        rmdir "$install_dir"
    fi
    printf '%s\n' 'TADX was removed. Configuration, workspaces, Guidance, and OS-stored credentials were preserved.'
    exit 0
fi

command -v curl >/dev/null 2>&1 || fail 'curl is required.'
command -v tar >/dev/null 2>&1 || fail 'tar is required.'

case "$(uname -s)" in
    Darwin) operating_system='darwin' ;;
    Linux) operating_system='linux' ;;
    *) fail "Unsupported operating system: $(uname -s). Use install.ps1 on Windows." ;;
esac

case "$(uname -m)" in
    x86_64|amd64) architecture='amd64' ;;
    arm64|aarch64) architecture='arm64' ;;
    *) fail "Unsupported architecture: $(uname -m). TADX supports amd64 and arm64." ;;
esac

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/tadx-install.XXXXXXXX")
cleanup() {
    rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

manifest_path="${temporary_directory}/checksums.txt"

if [ "$version" = 'latest' ]; then
    release_base="https://github.com/${repository}/releases/latest/download"
    curl -fL --proto '=https' --tlsv1.2 -o "$manifest_path" "${release_base}/checksums.txt"
    suffix="_${operating_system}_${architecture}.tar.gz"
    asset_name=$(awk -v suffix="$suffix" '
        length($1) == 64 && substr($2, length($2) - length(suffix) + 1) == suffix { print $2 }
    ' "$manifest_path")
    asset_count=$(printf '%s\n' "$asset_name" | awk 'NF { count++ } END { print count+0 }')
    [ "$asset_count" -eq 1 ] || fail "The latest stable release does not contain exactly one ${operating_system} ${architecture} archive."
    case "$asset_name" in
        tadx_*"$suffix") resolved_version=${asset_name#tadx_}; resolved_version=${resolved_version%"$suffix"} ;;
        *) fail 'The release asset name does not contain a version.' ;;
    esac
    printf '%s\n' "$resolved_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$' || fail "The release contains an invalid TADX version: $resolved_version"
else
    resolved_version=${version#v}
    printf '%s\n' "$resolved_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$' || fail "Invalid TADX version: $version"
    asset_name="tadx_${resolved_version}_${operating_system}_${architecture}.tar.gz"
    downloaded=0
    for tag in "$version" "v${resolved_version}" "$resolved_version"; do
        release_base="https://github.com/${repository}/releases/download/${tag}"
        if curl -fL --proto '=https' --tlsv1.2 -o "$manifest_path" "${release_base}/checksums.txt" 2>/dev/null; then
            downloaded=1
            break
        fi
        rm -f "$manifest_path"
    done
    [ "$downloaded" -eq 1 ] || fail "TADX release $version was not found."
fi

expected_hash=$(awk -v file="$asset_name" '
    length($1) == 64 && ($2 == file || $2 == "*" file) { print tolower($1) }
' "$manifest_path")
hash_count=$(printf '%s\n' "$expected_hash" | awk 'NF { count++ } END { print count+0 }')
[ "$hash_count" -eq 1 ] || fail "The checksum manifest must contain exactly one entry for $asset_name."

archive_path="${temporary_directory}/${asset_name}"
curl -fL --proto '=https' --tlsv1.2 -o "$archive_path" "${release_base}/${asset_name}"

if command -v sha256sum >/dev/null 2>&1; then
    actual_hash=$(sha256sum "$archive_path" | awk '{ print tolower($1) }')
elif command -v shasum >/dev/null 2>&1; then
    actual_hash=$(shasum -a 256 "$archive_path" | awk '{ print tolower($1) }')
elif command -v openssl >/dev/null 2>&1; then
    actual_hash=$(openssl dgst -sha256 "$archive_path" | awk '{ print tolower($NF) }')
else
    fail 'sha256sum, shasum, or openssl is required to verify the release archive.'
fi

[ "$actual_hash" = "$expected_hash" ] || fail "Checksum verification failed for $asset_name. The archive was not installed."

extract_directory="${temporary_directory}/extract"
mkdir "$extract_directory"
archive_members="${temporary_directory}/archive-members.txt"
tar -tzf "$archive_path" > "$archive_members"
while IFS= read -r member; do
    case "$member" in
        /*|../*|*/../*|*/..) fail 'The verified release archive contains an unsafe path.' ;;
    esac
done < "$archive_members"
tar -xzf "$archive_path" -C "$extract_directory"
binary_paths=$(find "$extract_directory" -type f -name tadx -print)
binary_count=$(printf '%s\n' "$binary_paths" | awk 'NF { count++ } END { print count+0 }')
[ "$binary_count" -eq 1 ] || fail 'The verified release archive must contain exactly one tadx file.'

mkdir -p "$install_dir"
staged_binary="${install_dir}/.tadx.new.$$"
cp "$binary_paths" "$staged_binary"
chmod 0755 "$staged_binary"
mv -f "$staged_binary" "${install_dir}/tadx"

if [ "$modify_path" -eq 1 ]; then
    add_managed_path
fi

printf 'TADX %s was installed at %s.\n' "$resolved_version" "${install_dir}/tadx"
if [ "$modify_path" -eq 0 ]; then
    printf 'Add %s to PATH to run tadx from any directory.\n' "$install_dir"
else
    printf '%s\n' 'Open a new terminal if the tadx command is not available in the current terminal.'
fi
