# Source this file in Bash to browse and change directory in the current shell.
WTF_PILOT_BINARY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/bin/wtf-pilot"

wtf-pilot() {
    local subcommand="${1,,}" argument destination
    case "$subcommand" in
        list|ls)
            if [[ -t 0 && -t 1 ]]; then
                for argument in "$@"; do
                    case "${argument,,}" in
                        --json*|-j*|--help*|-h*|-[^-]*[jh]*)
                            "$WTF_PILOT_BINARY" "$@"
                            return $?
                            ;;
                    esac
                done
                destination="$("$WTF_PILOT_BINARY" "$@" --select)" || return $?
                if [[ -n "$destination" ]]; then
                    builtin cd -- "$destination"
                    return $?
                fi
                return 0
            fi
            ;;
    esac
    "$WTF_PILOT_BINARY" "$@"
}
