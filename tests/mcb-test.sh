#!/usr/bin/env bash
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
cleanup() {
    rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT

TEST_ROOT=$TEMP_DIR/repository
FAKE_BIN=$TEMP_DIR/bin
mkdir -p "$TEST_ROOT/scripts" "$TEST_ROOT/extension" "$TEST_ROOT/secrets" "$FAKE_BIN"
cp "$ROOT/mcb" "$TEST_ROOT/mcb"
cp "$ROOT/scripts/bootstrap-compose.sh" "$TEST_ROOT/scripts/bootstrap-compose.sh"
touch "$TEST_ROOT/Dockerfile" "$TEST_ROOT/docker-compose.yml" "$TEST_ROOT/extension/manifest.json"
printf 'sentinel-master-key\n' >"$TEST_ROOT/secrets/master-key"
chmod 0700 "$TEST_ROOT/secrets"
chmod 0444 "$TEST_ROOT/secrets/master-key"

cat >"$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case " $* " in
    *' compose version '*) printf 'Docker Compose version v2.test\n' ;;
    *' --version '*) printf 'Docker version test\n' ;;
    *' info '*) exit 0 ;;
    *' image inspect '*) exit 0 ;;
    *' compose run '*' user list '*)
        if [[ -n ${FAKE_DOCKER_STATE:-} ]]; then
            if [[ -f $FAKE_DOCKER_STATE/browser-publisher ]]; then
                printf 'browser-publisher\tpublisher\tyoutube/default\n'
            fi
            if [[ -f $FAKE_DOCKER_STATE/downloader ]]; then
                printf 'downloader\treader\tyoutube/default\n'
            fi
        else
            printf 'browser-publisher\tpublisher\tyoutube/default\n'
            printf 'downloader\treader\tyoutube/default\n'
        fi
        ;;
    *' compose run '*' user add browser-publisher '*)
        [[ -n ${FAKE_DOCKER_STATE:-} && ! -e $FAKE_DOCKER_STATE/browser-publisher ]]
        touch "$FAKE_DOCKER_STATE/browser-publisher"
        printf 'created\n' >>"$FAKE_DOCKER_STATE/publisher-adds"
        printf 'Generated password for browser-publisher (shown once): fake-publisher-password\n'
        ;;
    *' compose run '*' user add downloader '*)
        [[ -n ${FAKE_DOCKER_STATE:-} && ! -e $FAKE_DOCKER_STATE/downloader ]]
        IFS= read -r reader_password
        [[ $reader_password == fake-reader-password ]]
        printf '%s\n' "$reader_password" >"$FAKE_DOCKER_STATE/reader-password-from-stdin"
        touch "$FAKE_DOCKER_STATE/downloader"
        printf 'created\n' >>"$FAKE_DOCKER_STATE/reader-adds"
        ;;
    *' run --rm --entrypoint brokerctl '*' generate-key '*)
        [[ -n ${FAKE_DOCKER_STATE:-} ]]
        printf 'generated\n' >>"$FAKE_DOCKER_STATE/reader-password-generations"
        printf 'fake-reader-password\n'
        ;;
    *' compose up -d broker '*) exit 0 ;;
    *' compose ps -q broker '*) printf 'fake-container\n' ;;
    *" inspect --format {{.State.Running}} fake-container "*) printf 'true\n' ;;
    *) printf 'unexpected fake docker command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$FAKE_BIN/docker"
cat >"$FAKE_BIN/curl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$FAKE_BIN/curl"

help_output=$(cd / && "$TEST_ROOT/mcb" help)
[[ $help_output == *'setup'* && $help_output == *'extension-path'* ]]

if (cd / && "$TEST_ROOT/mcb" unknown) >/dev/null 2>&1; then
    printf 'unknown command unexpectedly succeeded\n' >&2
    exit 1
fi

expected_extension=$TEST_ROOT/extension
actual_extension=$(cd / && "$TEST_ROOT/mcb" extension-path)
[[ $actual_extension == "$expected_extension" ]]

before=$(cksum "$TEST_ROOT/secrets/master-key")
PATH="$FAKE_BIN:$PATH" COOKIE_BROKER_URL=http://127.0.0.1:9 "$TEST_ROOT/mcb" setup >/dev/null
PATH="$FAKE_BIN:$PATH" COOKIE_BROKER_URL=http://127.0.0.1:9 "$TEST_ROOT/mcb" setup >/dev/null
after=$(cksum "$TEST_ROOT/secrets/master-key")
[[ $before == "$after" ]]

FIRST_RUN_ROOT=$TEMP_DIR/first-run-repository
FIRST_RUN_STATE=$TEMP_DIR/first-run-state
mkdir -p "$FIRST_RUN_ROOT/scripts" "$FIRST_RUN_ROOT/extension" "$FIRST_RUN_ROOT/secrets" "$FIRST_RUN_STATE"
cp "$ROOT/mcb" "$FIRST_RUN_ROOT/mcb"
cp "$ROOT/scripts/bootstrap-compose.sh" "$FIRST_RUN_ROOT/scripts/bootstrap-compose.sh"
touch "$FIRST_RUN_ROOT/Dockerfile" "$FIRST_RUN_ROOT/docker-compose.yml" "$FIRST_RUN_ROOT/extension/manifest.json"
printf 'first-run-sentinel-master-key\n' >"$FIRST_RUN_ROOT/secrets/master-key"
chmod 0700 "$FIRST_RUN_ROOT/secrets"
chmod 0444 "$FIRST_RUN_ROOT/secrets/master-key"

first_master_before=$(cksum "$FIRST_RUN_ROOT/secrets/master-key")
PATH="$FAKE_BIN:$PATH" FAKE_DOCKER_STATE="$FIRST_RUN_STATE" \
    "$FIRST_RUN_ROOT/mcb" setup >"$TEMP_DIR/first-run.log"

grep -q 'password: fake-publisher-password' "$TEMP_DIR/first-run.log"
[[ -f $FIRST_RUN_STATE/browser-publisher ]]
[[ -f $FIRST_RUN_STATE/downloader ]]
[[ $(wc -l <"$FIRST_RUN_STATE/publisher-adds") == 1 ]]
[[ $(wc -l <"$FIRST_RUN_STATE/reader-adds") == 1 ]]
[[ $(wc -l <"$FIRST_RUN_STATE/reader-password-generations") == 1 ]]
[[ $(<"$FIRST_RUN_STATE/reader-password-from-stdin") == fake-reader-password ]]
[[ $(<"$FIRST_RUN_ROOT/secrets/reader-password") == fake-reader-password ]]
[[ $(stat -c '%a' "$FIRST_RUN_ROOT/secrets/reader-password") == 600 ]]

reader_before=$(cksum "$FIRST_RUN_ROOT/secrets/reader-password")
PATH="$FAKE_BIN:$PATH" FAKE_DOCKER_STATE="$FIRST_RUN_STATE" \
    "$FIRST_RUN_ROOT/mcb" setup >"$TEMP_DIR/first-run-repeat.log"

grep -q 'Publisher already exists' "$TEMP_DIR/first-run-repeat.log"
grep -q 'Existing reader password file preserved' "$TEMP_DIR/first-run-repeat.log"
[[ $(wc -l <"$FIRST_RUN_STATE/publisher-adds") == 1 ]]
[[ $(wc -l <"$FIRST_RUN_STATE/reader-adds") == 1 ]]
[[ $(wc -l <"$FIRST_RUN_STATE/reader-password-generations") == 1 ]]
[[ $(cksum "$FIRST_RUN_ROOT/secrets/reader-password") == "$reader_before" ]]
[[ $(cksum "$FIRST_RUN_ROOT/secrets/master-key") == "$first_master_before" ]]

PATH="$FAKE_BIN:$PATH" "$TEST_ROOT/mcb" doctor >"$TEMP_DIR/doctor-warning.log"
grep -q 'Result: healthy' "$TEMP_DIR/doctor-warning.log"
chmod 0600 "$TEST_ROOT/secrets/master-key"
: >"$TEST_ROOT/secrets/master-key"
if PATH="$FAKE_BIN:$PATH" "$TEST_ROOT/mcb" doctor >/dev/null; then
    printf 'doctor unexpectedly accepted an empty master key\n' >&2
    exit 1
fi

mv "$TEST_ROOT/secrets/master-key" "$TEST_ROOT/secrets/empty-master-key"
printf 'external-sentinel\n' >"$TEMP_DIR/external-key"
external_before=$(cksum "$TEMP_DIR/external-key")
ln -s "$TEMP_DIR/external-key" "$TEST_ROOT/secrets/master-key"
if PATH="$FAKE_BIN:$PATH" "$TEST_ROOT/scripts/bootstrap-compose.sh" >/dev/null 2>&1; then
    printf 'bootstrap unexpectedly accepted a symlinked master key\n' >&2
    exit 1
fi
external_after=$(cksum "$TEMP_DIR/external-key")
[[ $external_before == "$external_after" ]]

printf 'mcb shell tests passed\n'
