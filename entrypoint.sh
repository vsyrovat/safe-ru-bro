#!/bin/sh
set -eu

if [ "$(id -u)" -eq 0 ]; then
    uid=${HOST_UID:-1000}
    gid=${HOST_GID:-1000}
    groups=${HOST_GROUPS:-$gid}
    echo "browser:x:${uid}:${gid}:Browser:/home/browser:/bin/sh" >> /etc/passwd
    echo "browser:x:${gid}:" >> /etc/group
    if [ ! -s /etc/machine-id ]; then
        dbus-uuidgen --ensure || true
    fi
    mkdir -p /run/dbus /etc/chromium/policies/recommended
    dbus-daemon --system --fork || true
    # Default Override CPU performance tier to TIER0: UNKNOWN. A choice saved
    # in the profile overrides this.
    cat > /etc/chromium/policies/recommended/cpu.json <<'EOF'
{
  "CpuPerformanceTierOverride": 0
}
EOF
    if [ -n "${PROXY_HOST:-}" ]; then
        /usr/local/bin/safe-ru-bro tunnel &
        i=0
        while [ "$i" -lt 50 ]; do
            if ip -4 route show default | grep -q 'tun0'; then
                break
            fi
            i=$((i + 1))
            sleep 0.1
        done
        if ! ip -4 route show default | grep -q 'tun0'; then
            echo "safe-ru-bro: proxy tunnel did not start" >&2
            exit 1
        fi
    fi
    exec setpriv --reuid="$uid" --regid="$gid" --groups "$groups" --inh-caps=-all \
        /usr/local/bin/entrypoint.sh "$@"
fi

export HOME="${HOME:-/home/browser}"
if [ -z "${XDG_RUNTIME_DIR:-}" ]; then
    XDG_RUNTIME_DIR=/tmp/runtime
    export XDG_RUNTIME_DIR
fi
DATA_DIR=/home/browser/data
CACHE_DIR=/home/browser/cache
mkdir -p "$DATA_DIR/Default" "$CACHE_DIR" "$HOME/.pki/nssdb" "$HOME/Downloads" "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"

# HTTP cache uses --disk-cache-dir. These other caches live inside the profile
# by default; keep them in the cache mount so data/ stays cookies and site storage.
relocate_cache() {
    rel=$1
    dest_name=$(printf '%s' "$rel" | tr '/' '_')
    dest="$CACHE_DIR/$dest_name"
    src="$DATA_DIR/$rel"
    case "$rel" in
        */*) link_target="../../cache/$dest_name" ;;
        *) link_target="../cache/$dest_name" ;;
    esac
    mkdir -p "$(dirname "$src")" "$dest"
    if [ -L "$src" ]; then
        return 0
    fi
    if [ -d "$src" ]; then
        find "$src" -mindepth 1 -maxdepth 1 -exec mv {} "$dest"/ \;
        rmdir "$src"
    elif [ -e "$src" ]; then
        mv "$src" "$dest"/
    fi
    ln -s "$link_target" "$src"
}

for rel in \
    "Default/GPUCache" \
    "Default/DawnGraphiteCache" \
    "Default/DawnWebGPUCache" \
    "Default/Shared Dictionary" \
    "GPUPersistentCache" \
    "ShaderCache" \
    "GrShaderCache" \
    "GraphiteDawnCache"
do
    relocate_cache "$rel"
done

# Ungoogled Chromium trusts locally imported NSS roots. GOST certificates stay in the
# system bundle only; NSS in this image cannot store them.
if [ ! -e "$HOME/.pki/nssdb/cert9.db" ]; then
    certutil -d "sql:$HOME/.pki/nssdb" -N --empty-password
    for cert in /usr/local/share/ca-certificates/russian/*.crt; do
        name=$(basename "$cert" .crt)
        case "$name" in
            *gost*) continue ;;
        esac
        certutil -d "sql:$HOME/.pki/nssdb" -A -t "C,," -n "$name" -i "$cert"
    done
fi

# Reopen the previous tabs. Ungoogled Chromium does this only after a clean quit;
# a crash opens a new tab so a bad page cannot lock the browser in a loop.
# dbus-run-session does not forward SIGTERM, so stopping the container looked
# like a crash and the saved tabs were dropped.
eval "$(dbus-launch --sh-syntax)"

browser_pid=
requested=0
forward_stop() {
    requested=1
    if [ -n "$browser_pid" ]; then
        kill -TERM "$browser_pid" 2>/dev/null || true
    fi
}
trap forward_stop TERM INT

# A profile Chromium has never written is a new installation.
if [ ! -e "$DATA_DIR/Local State" ]; then
    set -- "https://browserleaks.com" "$@"
fi

/opt/ungoogled-chromium/chrome-wrapper \
    --no-first-run \
    --no-default-browser-check \
    --disable-sync \
    --password-store=basic \
    --lang=ru \
    --restore-last-session \
    --user-data-dir="$DATA_DIR" \
    --disk-cache-dir="$CACHE_DIR" \
    "$@" &
browser_pid=$!

status=0
while kill -0 "$browser_pid" 2>/dev/null; do
    wait "$browser_pid" && status=0 || status=$?
done

trap - TERM INT
if [ -n "${DBUS_SESSION_BUS_PID:-}" ]; then
    kill -TERM "$DBUS_SESSION_BUS_PID" 2>/dev/null || true
fi
if [ "$requested" -eq 1 ]; then
    exit 0
fi
exit "$status"
