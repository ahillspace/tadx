#!/bin/sh

set -eu

repository='ahillspace/tadx'
path_marker='# tadx-installer-path'
action='install'
version=${TADX_VERSION:-latest}
install_dir=${TADX_INSTALL_DIR:-"${HOME}/.local/bin"}
modify_path=1
completion=1
completion_marker='# tadx-installer-completion'
targets=''

usage() {
    printf '%s\n' 'Usage: install.sh [install|uninstall] [--version VERSION] [--target TARGET] [--install-dir DIRECTORY] [--no-modify-path] [--no-completion]'
    printf '%s\n' '  --version VERSION       install an exact release; default: latest'
    printf '%s\n' '  --target TARGET         install bundled Guidance for one target; repeat for more than one'
    printf '%s\n' '  --install-dir DIRECTORY resolved binary directory; default: $TADX_INSTALL_DIR or $HOME/.local/bin'
    printf '%s\n' '  --no-modify-path        leave shell profiles and PATH unchanged'
    printf '%s\n' '  --no-completion         leave shell completion profiles unchanged'
    printf '%s\n' 'For a local install without profile edits: install.sh --no-modify-path --no-completion'
    printf '%s\n' 'Completion is enabled for the current Bash, Zsh, or Fish shell. Open a new shell to load it.'
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
        --target)
            [ "$#" -ge 2 ] || fail '--target requires a value.'
            case "$2" in ''|*[!a-z0-9-]*) fail 'Invalid agent target.' ;; esac
            targets="${targets} $2"
            shift 2
            ;;
        --no-modify-path)
            modify_path=0
            shift
            ;;
        --no-completion)
            completion=0
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
        */zsh) printf '%s\n' "${ZDOTDIR:-$HOME}/.zshrc" ;;
        */bash) printf '%s\n' "${HOME}/.bashrc" ;;
        */fish) printf '%s\n' "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish" ;;
        *) printf '%s\n' "${HOME}/.profile" ;;
    esac
}

prepare_profile() {
    profile=$(profile_path)
    mkdir -p "$(dirname "$profile")"
    if [ -f "$profile" ] && [ ! -e "${profile}.tadx-backup" ]; then
        cp -p "$profile" "${profile}.tadx-backup"
    fi
    if [ ! -e "$profile" ]; then
        (umask 077; : > "$profile")
    fi
    if [ -s "$profile" ] && [ -n "$(tail -c 1 "$profile")" ]; then
        printf '\n' >> "$profile"
    fi
}

remove_managed_completion() {
    profile=$(profile_path)
    [ -f "$profile" ] || return 0
    grep -Fq "$completion_marker" "$profile" || return 0
    prepare_profile
    temporary_profile="${profile}.tadx.$$"
    cp -p "$profile" "$temporary_profile"
    awk -v marker="$completion_marker" 'substr($0, length($0)-length(marker)+1) != marker' "$profile" > "$temporary_profile"
    mv -f "$temporary_profile" "$profile"
}

add_managed_completion() {
    case "${SHELL:-}" in
        */bash|*/zsh|*/fish) ;;
        *) printf '%s\n' 'Completion setup skipped: use tadx completion --help for supported shells.'; return 0 ;;
    esac
    prepare_profile
    remove_managed_completion
    escaped_binary=$(printf '%s' "${install_dir}/tadx" | sed "s/'/'\\\\''/g")
    case "$SHELL" in
        */bash) printf "[ ! -x '%s' ] || source <('%s' completion bash) %s\n" "$escaped_binary" "$escaped_binary" "$completion_marker" >> "$profile" ;;
        */zsh) printf "if [[ -x '%s' ]]; then autoload -Uz compinit; (( \$+functions[compdef] )) || compinit; source <('%s' completion zsh); fi %s\n" "$escaped_binary" "$escaped_binary" "$completion_marker" >> "$profile" ;;
        */fish)
            escaped_binary=$(printf '%s' "${install_dir}/tadx" | sed "s/\\\\/\\\\\\\\/g; s/'/\\\\'/g")
            printf "if test -x '%s'; '%s' completion fish | source; end %s\n" "$escaped_binary" "$escaped_binary" "$completion_marker" >> "$profile"
            ;;
    esac
    printf 'Completion is enabled in %s. Open a new shell to load it.\n' "$profile"
}

remove_managed_path() {
    profile=$(profile_path)
    [ -f "$profile" ] || return 0
    grep -Fq "$path_marker" "$profile" || return 0
    prepare_profile
    temporary_profile="${profile}.tadx.$$"
    cp -p "$profile" "$temporary_profile"
    awk -v marker="$path_marker" 'substr($0, length($0)-length(marker)+1) != marker' "$profile" > "$temporary_profile"
    mv -f "$temporary_profile" "$profile"
}

add_managed_path() {
    profile=$(profile_path)
    if ! { [ -f "$profile" ] && grep -Fq "$path_marker" "$profile"; }; then
        case ":${PATH}:" in
            *":${install_dir}:"*) return 0 ;;
        esac
    fi

    prepare_profile
    remove_managed_path

    escaped_dir=$(printf '%s' "$install_dir" | sed "s/'/'\\\\''/g")
    case "${SHELL:-}" in
        */fish)
            escaped_dir=$(printf '%s' "$install_dir" | sed "s/\\\\/\\\\\\\\/g; s/'/\\\\'/g")
            printf "if not contains -- '%s' \$PATH; set -gx PATH '%s' \$PATH; end %s\n" "$escaped_dir" "$escaped_dir" "$path_marker" >> "$profile" ;;
        *) printf "export PATH='%s':\"\$PATH\" %s\n" "$escaped_dir" "$path_marker" >> "$profile" ;;
    esac
}

if [ "$action" = 'uninstall' ]; then
    rm -f "${install_dir}/tadx"
    if [ "$modify_path" -eq 1 ]; then remove_managed_path; fi
    if [ "$completion" -eq 1 ]; then remove_managed_completion; fi
    if [ -d "$install_dir" ] && [ -z "$(ls -A "$install_dir")" ]; then
        rmdir "$install_dir"
    fi
    printf '%s\n' 'TADX was removed. Configuration, workspaces, Guidance, and OS-stored credentials were preserved.'
    exit 0
fi

command -v tar >/dev/null 2>&1 || fail 'tar is required.'

use_github_cli=0
if command -v gh >/dev/null 2>&1 && gh auth status --hostname github.com >/dev/null 2>&1; then
    use_github_cli=1
fi

receive_release_asset() {
    release_tag=$1
    release_asset=$2
    release_destination=$3
    release_https_base=$4
    rm -f "$release_destination"

    if [ "$use_github_cli" -eq 1 ]; then
        if [ -n "$release_tag" ]; then
            if gh release download "$release_tag" --repo "$repository" --pattern "$release_asset" --output "$release_destination" >/dev/null 2>&1; then
                [ -f "$release_destination" ] && return 0
            fi
        elif gh release download --repo "$repository" --pattern "$release_asset" --output "$release_destination" >/dev/null 2>&1; then
            [ -f "$release_destination" ] && return 0
        fi
        rm -f "$release_destination"
    fi

    command -v curl >/dev/null 2>&1 || return 1
    http_status=$(curl -sSL --proto '=https' --tlsv1.2 --connect-timeout 15 --max-time 120 --max-filesize 268435456 -o "$release_destination" -w '%{http_code}' "${release_https_base}/${release_asset}" 2>/dev/null) || {
        rm -f "$release_destination"
        return 1
    }
    if [ -n "$http_status" ]; then
        case "$http_status" in
            404) rm -f "$release_destination"; return 2 ;;
            2??) ;;
            *) rm -f "$release_destination"; return 1 ;;
        esac
    fi
    [ -f "$release_destination" ] || return 1
}

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
owns_install_lock=0
cleanup() {
    if [ "$owns_install_lock" -eq 1 ]; then rmdir "${install_dir}/.tadx-install.lock"; fi
    rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

manifest_path="${temporary_directory}/checksums.txt"

if [ "$version" = 'latest' ]; then
    release_base="https://github.com/${repository}/releases/latest/download"
    release_tag=''
    if receive_release_asset "$release_tag" 'checksums.txt' "$manifest_path" "$release_base"; then
        :
    else
        download_status=$?
        case "$download_status" in
            2) fail 'The latest stable TADX release was not found (asset checksums.txt). It was not verified or installed.' ;;
            *) fail 'The latest stable TADX release could not be retrieved (asset checksums.txt; bounded transport failure). It was not verified or installed.' ;;
        esac
    fi
    [ "$(wc -c < "$manifest_path")" -le 1048576 ] || fail 'The release checksum manifest exceeds its byte limit.'
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
    transport_failure=0
    not_found=0
    for tag in "$version" "v${resolved_version}" "$resolved_version"; do
        release_base="https://github.com/${repository}/releases/download/${tag}"
        if receive_release_asset "$tag" 'checksums.txt' "$manifest_path" "$release_base" 2>/dev/null; then
            release_tag=$tag
            downloaded=1
            break
        else
            case "$?" in
                2) not_found=1 ;;
                *) transport_failure=1 ;;
            esac
        fi
        rm -f "$manifest_path"
    done
    if [ "$downloaded" -ne 1 ]; then
        if [ "$transport_failure" -eq 1 ]; then
            fail "TADX release $version could not be retrieved (asset checksums.txt; bounded transport failure). It was not verified or installed."
        fi
        [ "$not_found" -eq 1 ] || fail "TADX release $version could not be retrieved (asset checksums.txt). It was not verified or installed."
        fail "TADX release $version was not found (asset checksums.txt). It was not verified or installed."
    fi
fi

 [ "$(wc -c < "$manifest_path")" -le 1048576 ] || fail 'The release checksum manifest exceeds its byte limit.'
expected_hash=$(awk -v file="$asset_name" '
    length($1) == 64 && ($2 == file || $2 == "*" file) { print tolower($1) }
' "$manifest_path")
hash_count=$(printf '%s\n' "$expected_hash" | awk 'NF { count++ } END { print count+0 }')
[ "$hash_count" -eq 1 ] || fail "The checksum manifest must contain exactly one entry for $asset_name."

archive_path="${temporary_directory}/${asset_name}"
if receive_release_asset "$release_tag" "$asset_name" "$archive_path" "$release_base"; then
    download_status=0
else
    download_status=$?
fi
case "$download_status" in
    0) ;;
    2) fail "The release asset $asset_name for version ${resolved_version:-$version} was not found. It was not verified or installed." ;;
    *) fail "The release asset $asset_name for version ${resolved_version:-$version} could not be retrieved (bounded transport failure). It was not verified or installed." ;;
esac
[ "$(wc -c < "$archive_path")" -le 268435456 ] || fail 'The release archive exceeds its byte limit.'

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

mkdir -p "$install_dir" || fail "Could not create the resolved install directory '$install_dir'. No replacement occurred; choose another --install-dir or check directory permissions."
mkdir "${install_dir}/.tadx-install.lock" 2>/dev/null || fail 'Another installer is using this directory. Wait for it to finish; remove .tadx-install.lock only after confirming no installer is running.'
owns_install_lock=1
staged_binary="${install_dir}/.tadx.new.$$"
cp "$binary_paths" "$staged_binary" || { rm -f "$staged_binary"; fail "Could not stage the release binary at '$staged_binary'. No replacement occurred; choose another --install-dir or check directory permissions."; }
chmod 0755 "$staged_binary" || { rm -f "$staged_binary"; fail "Could not set executable permissions for the staged binary at '$staged_binary'. No replacement occurred; choose another --install-dir or check directory permissions."; }
backup_binary="${install_dir}/.tadx.backup.$$"
if [ -e "${install_dir}/tadx" ]; then
    cp -p "${install_dir}/tadx" "$backup_binary" || { rm -f "$backup_binary" "$staged_binary"; fail "Could not preserve the existing binary at '${install_dir}/tadx'. No replacement occurred; choose another --install-dir or check directory permissions."; }
fi
mv -f "$staged_binary" "${install_dir}/tadx" || {
    rm -f "$staged_binary"
    if [ -f "$backup_binary" ]; then mv -f "$backup_binary" "${install_dir}/tadx" 2>/dev/null || true; fi
    fail "Could not replace the resolved binary at '${install_dir}/tadx'. The previous binary was restored when possible; choose another --install-dir or check directory permissions."
}
for target in ${targets:-auto}; do
if ! "${install_dir}/tadx" agent install --target "$target"; then
    if [ -f "$backup_binary" ]; then
        mv -f "$backup_binary" "${install_dir}/tadx"
    else
        rm -f "${install_dir}/tadx"
    fi
    fail 'Guidance installation failed. The binary was rolled back; any completed Guidance targets were reported above. Retry the installer to complete setup.'
fi
done

if [ "$modify_path" -eq 1 ]; then
    add_managed_path
fi
if [ "$completion" -eq 1 ]; then
    add_managed_completion
fi

printf 'TADX %s was installed at %s.\n' "$resolved_version" "${install_dir}/tadx"
if [ "$modify_path" -eq 0 ]; then
    printf 'Add %s to PATH to run tadx from any directory.\n' "$install_dir"
else
    printf '%s\n' 'Open a new terminal if the tadx command is not available in the current terminal.'
fi
