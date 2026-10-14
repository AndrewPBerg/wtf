# Bash navigation for the installed WTF CLI. Source this file in the current shell.
wtf() {
    local binary="${WTF_BINARY:-$HOME/.local/bin/wtf}"
    local subcommand="${1,,}" argument destination
    case "$subcommand" in
        list|ls|sw)
            if [[ -t 0 && -t 1 ]]; then
                for argument in "$@"; do
                    case "${argument,,}" in
                        --json=false|-j=false|--help=false|-h=false)
                            ;;
                        --json*|-j*|--help*|-h*|-[^-]*[jh]*|--select*)
                            "$binary" "$@"
                            return $?
                            ;;
                    esac
                done
                destination="$("$binary" "$@" --select)" || return $?
                if [[ -n "$destination" ]]; then
                    builtin cd -- "$destination"
                    return $?
                fi
                return 0
            fi
            ;;
    esac
    "$binary" "$@"
}
