#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
cleanup() {
    rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT

FIXTURE_ROOT=$TEMP_DIR/media-cookie-broker-main
FAKE_BIN=$TEMP_DIR/bin
FAKE_STATE=$TEMP_DIR/state
XDG_ROOT=$TEMP_DIR/data
INSTALL_TMP=$TEMP_DIR/installer-tmp
mkdir -p "$FIXTURE_ROOT/scripts" "$FAKE_BIN" "$FAKE_STATE" "$XDG_ROOT" "$INSTALL_TMP"
cp "$ROOT/mcb" "$FIXTURE_ROOT/mcb"
cp "$ROOT/docker-compose.yml" "$FIXTURE_ROOT/docker-compose.yml"
cp "$ROOT/scripts/bootstrap-compose.sh" "$FIXTURE_ROOT/scripts/bootstrap-compose.sh"
touch "$FIXTURE_ROOT/Dockerfile"
mkdir -p "$FIXTURE_ROOT/extension" "$FIXTURE_ROOT/demo" "$FIXTURE_ROOT/tests" \
    "$FIXTURE_ROOT/cmd" "$FIXTURE_ROOT/internal" "$FIXTURE_ROOT/docs"
touch "$FIXTURE_ROOT/README.md" "$FIXTURE_ROOT/go.mod" "$FIXTURE_ROOT/cmd/main.go"
touch "$FIXTURE_ROOT/extension/manifest.json" "$FIXTURE_ROOT/demo/video.ts" "$FIXTURE_ROOT/tests/source-test.sh"
tar -czf "$TEMP_DIR/source.tar.gz" -C "$TEMP_DIR" media-cookie-broker-main

cat >"$FAKE_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=
while (($#)); do
    if [[ $1 == -o ]]; then
        output=$2
        shift 2
    else
        shift
    fi
done
if [[ -n $output ]]; then
    cp "$FIXTURE_ARCHIVE" "$output"
fi
EOF

cat >"$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_DOCKER_STATE/commands"
case " $* " in
    *' compose version '*) printf 'Docker Compose version v2.test\n' ;;
    *' --version '*) printf 'Docker version test\n' ;;
    *' info '*) exit 0 ;;
    *' build --target broker -t media-cookie-broker:preview '*) touch "$FAKE_DOCKER_STATE/image-built" ;;
    *' image inspect media-cookie-broker:preview '*) [[ -f $FAKE_DOCKER_STATE/image-built ]] ;;
    *' run --rm --entrypoint brokerctl media-cookie-broker:preview generate-key '*) printf 'fake-generated-key\n' ;;
    *' compose run '*' user list '*)
        if [[ -f $FAKE_DOCKER_STATE/publisher ]]; then
            printf 'browser-publisher\tpublisher\tyoutube/default\n'
        fi
        if [[ -f $FAKE_DOCKER_STATE/reader ]]; then
            printf 'downloader\treader\tyoutube/default\n'
        fi
        ;;
    *' compose run '*' user add browser-publisher '*)
        touch "$FAKE_DOCKER_STATE/publisher"
        printf 'Generated password for browser-publisher (shown once): fake-publisher-password\n'
        ;;
    *' compose run '*' user add downloader '*)
        IFS= read -r password
        [[ $password == fake-generated-key ]]
        touch "$FAKE_DOCKER_STATE/reader"
        ;;
    *' compose up -d broker '*) exit 0 ;;
    *' compose ps -q broker '*) printf 'fake-container\n' ;;
    *" inspect --format {{.State.Running}} fake-container "*) printf 'true\n' ;;
    *) printf 'unexpected fake docker command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$FAKE_BIN/curl" "$FAKE_BIN/docker"

PATH="$FAKE_BIN:$PATH" \
    FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
    FAKE_DOCKER_STATE="$FAKE_STATE" \
    XDG_DATA_HOME="$XDG_ROOT" \
    TMPDIR="$INSTALL_TMP" \
    "$ROOT/install.sh" >"$TEMP_DIR/install.log"

INSTALL_ROOT=$XDG_ROOT/media-cookie-broker
[[ -x $INSTALL_ROOT/mcb ]]
[[ -f $INSTALL_ROOT/docker-compose.yml ]]
[[ -x $INSTALL_ROOT/scripts/bootstrap-compose.sh ]]
[[ -s $INSTALL_ROOT/secrets/master-key ]]
[[ -s $INSTALL_ROOT/secrets/reader-password ]]
[[ $(stat -c '%a' "$INSTALL_ROOT/secrets/reader-password") == 600 ]]

for absent in extension demo docs tests cmd internal README.md go.mod Dockerfile; do
    [[ ! -e $INSTALL_ROOT/$absent ]]
done
if find "$INSTALL_ROOT" -type f -name '*.go' -print -quit | grep -q .; then
    printf 'installed runtime unexpectedly contains Go source\n' >&2
    exit 1
fi
! grep -qi 'extension path\|/extension' "$TEMP_DIR/install.log"
! grep -Eq '(^|[^[:alnum:]_])sudo([^[:alnum:]_]|$)|systemctl|crontab|chrome://|\.bashrc|\.zshrc' "$ROOT/install.sh"
[[ $(wc -l <"$ROOT/install-server.sh") -lt 20 ]]
[[ -z $(find "$INSTALL_TMP" -mindepth 1 -print -quit) ]]

build_line=$(grep -n '^build --target broker -t media-cookie-broker:preview ' "$FAKE_STATE/commands" | head -n1 | cut -d: -f1)
setup_line=$(grep -n 'compose run .* user list' "$FAKE_STATE/commands" | head -n1 | cut -d: -f1)
[[ -n $build_line && -n $setup_line && $build_line -lt $setup_line ]]

PATH="$FAKE_BIN:$PATH" FAKE_DOCKER_STATE="$FAKE_STATE" \
    "$INSTALL_ROOT/mcb" doctor >"$TEMP_DIR/doctor.log"
grep -q 'Result: healthy' "$TEMP_DIR/doctor.log"
! grep -qi 'extension' "$TEMP_DIR/doctor.log"

PATH="$FAKE_BIN:$PATH" FAKE_DOCKER_STATE="$FAKE_STATE" \
    "$INSTALL_ROOT/mcb" status >"$TEMP_DIR/status.log"
! grep -qi 'extension' "$TEMP_DIR/status.log"

master_before=$(cksum "$INSTALL_ROOT/secrets/master-key")
reader_before=$(cksum "$INSTALL_ROOT/secrets/reader-password")
PATH="$FAKE_BIN:$PATH" \
    FIXTURE_ARCHIVE="$TEMP_DIR/source.tar.gz" \
    FAKE_DOCKER_STATE="$FAKE_STATE" \
    XDG_DATA_HOME="$XDG_ROOT" \
    TMPDIR="$INSTALL_TMP" \
    "$ROOT/install.sh" >"$TEMP_DIR/reinstall.log"
grep -q 'Existing Media Cookie Broker installation found' "$TEMP_DIR/reinstall.log"
[[ $(cksum "$INSTALL_ROOT/secrets/master-key") == "$master_before" ]]
[[ $(cksum "$INSTALL_ROOT/secrets/reader-password") == "$reader_before" ]]
[[ $(grep -c '^build --target broker ' "$FAKE_STATE/commands") == 1 ]]
printf 'installer shell tests passed\n'
