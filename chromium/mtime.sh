#!/bin/bash
## mtime.sh
## @author csagan5
## @url https://github.com/bromite/mtool
##
## Script to backup/verify/restore mtime of files across Chromium source checkouts.
## Run it from the root git repository.
##
#

set -e

function usage() {
	echo "Usage: mtime.sh (--backup | --verify | --restore)" 1>&2
}

if [ ! $# -eq 1 ]; then
	usage
	exit 1
fi

case "$1" in
  --backup)
    MTOOL="mtool --snapshot=.mtool --create"
    ;;
  --verify)
    MTOOL="mtool --snapshot=.mtool --verify"
    ;;
  --restore)
    MTOOL="mtool --snapshot=.mtool --restore"
    ;;
  *)
    usage
    exit 1
esac

# temporary file for all git repositories
GITTMP="$(mktemp)"
# commands to run to take a snapshot of mtimes
CMDTMP="$(mktemp)"
trap "rm '$GITTMP' '$CMDTMP'" EXIT

# find all git directories
find -type d -name .git | awk '{ print substr($0, 3, length($0)-7) }' | grep -v ^$ > "$GITTMP"

# first the root directory
echo "git ls-files --exclude-standard --stage | grep -vF third_party/WebKit/LayoutTests/ | %MTOOL%" > "$CMDTMP"

# contains some common exclusion patterns for Chromium e.g. broken links and similar
awk '{ printf "cd %s && git ls-files --exclude-standard --stage | grep -vF /test/ | grep -vF test/data | grep -vF tools/gyp | grep -vF ui/src/gen | grep -vF example/payload | grep -vF static_test_env/ | %%MTOOL%% || echo \"%s: FAILED\"\n", $0, $0 }' "$GITTMP" >> "$CMDTMP"

sed -i "s/%MTOOL%/${MTOOL}/g" "$CMDTMP"

# run all commands in parallel
# add --will-cite after making a donation of at least 10k EUR to the author of parallel (see man parallel)
parallel < "$CMDTMP"
