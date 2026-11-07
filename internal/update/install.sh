#!/bin/bash
# Adapted from Notted (internal/integration/install/install.sh); see NOTICE.md.
# Puts a newly built Deputy.app in place of the installed one after the running Deputy quits.
# Usage: install.sh <pid> <new-app> <installed-app>
# It copies the new app beside the installed one, deletes the new app it was given, swaps the two
# with renames, and keeps the old copy until the new one is running. Every failure leaves an app at
# <installed-app> that opens. Deputy asks macOS for no privacy permissions, so a signature change
# needs no permission reset.
# Environment (all optional): DEPUTY_OPEN, DEPUTY_MV, DEPUTY_QUIT_WAIT (seconds, default 60),
# DEPUTY_LAUNCH_WAIT (default 20), DEPUTY_LAUNCH_SETTLE (default 2).
set -u
pid=$1
new=$2
dest=$3
open_cmd=${DEPUTY_OPEN:-/usr/bin/open}
mv_cmd=${DEPUTY_MV:-/bin/mv}
quit_wait=${DEPUTY_QUIT_WAIT:-60}
launch_wait=${DEPUTY_LAUNCH_WAIT:-20}
settle=${DEPUTY_LAUNCH_SETTLE:-2}
staged="$dest.new"
old="$dest.old"
exe="$dest/Contents/MacOS/Deputy"
here=$(cd "$(dirname "$0")" && pwd)
cleanup() { case "$here" in */deputy-install-*) rm -rf "$here" ;; esac; }
trap cleanup EXIT

log() { echo "install: $*"; }
# matching_pids prints the pid of every process running the installed executable.
matching_pids() {
  local procs line pid rest
  procs=$(ps -axo pid=,comm=) || return 0
  while IFS= read -r line; do
    line=${line#"${line%%[![:space:]]*}"}
    pid=${line%% *}
    rest=${line#* }
    rest=${rest#"${rest%%[![:space:]]*}"}
    [ "$rest" = "$exe" ] && printf '%s ' "$pid"
  done <<<"$procs"
  return 0
}
# running succeeds when a process that was not running before the open runs the new executable.
running() {
  local p
  for p in $(matching_pids); do
    case "$before" in *" $p "*) ;; *) return 0 ;; esac
  done
  return 1
}
before=" "
# restore_failed is the end of a rollback that could not put the previous version back.
restore_failed() {
  log "could not restore the previous version; it is at $old. Move it back to $dest in Finder, then open it."
  exit 1
}
# put_back <from> <to>: a rename never lands on an existing path, because mv would move into it.
put_back() {
  [ ! -e "$2" ] && [ ! -L "$2" ] || return 1
  "$mv_cmd" "$1" "$2"
}
gone() { ! kill -0 "$pid" 2>/dev/null; }
reopen() {
  log "reopening $dest"
  "$open_cmd" "$dest"
}

# wait_until <seconds> <command...>: succeeds as soon as the command does, fails after the time.
wait_until() {
  local n=$(($1 * 5))
  shift
  while [ "$n" -gt 0 ]; do
    "$@" && return 0
    sleep 0.2
    n=$((n - 1))
  done
  "$@"
}

log "waiting for Deputy (pid $pid) to quit"
if ! wait_until "$quit_wait" gone; then
  log "Deputy did not quit within $quit_wait seconds; nothing changed"
  rm -rf "$new"
  exit 1
fi
if [ -e "$old" ]; then
  log "$old is left from an earlier update; nothing changed"
  rm -rf "$new"
  reopen
  exit 1
fi
rm -rf "$staged"
log "copying $new to $staged"
if ! /usr/bin/ditto "$new" "$staged"; then
  rm -rf "$staged" "$new"
  log "copy failed; nothing changed"
  reopen
  exit 1
fi
# The copy beside the installed app is the only one needed from here on.
rm -rf "$new"
log "moving $dest to $old"
if ! "$mv_cmd" "$dest" "$old"; then
  rm -rf "$staged"
  log "rename failed; nothing changed"
  reopen
  exit 1
fi
log "moving $staged to $dest"
if ! "$mv_cmd" "$staged" "$dest"; then
  put_back "$old" "$dest" || restore_failed
  rm -rf "$staged"
  log "rename failed; restored $dest"
  reopen
  exit 1
fi
log "opening $dest"
before=" $(matching_pids)"
"$open_cmd" "$dest"
if wait_until "$launch_wait" running && sleep "$settle" && running; then
  rm -rf "$old"
  log "done; removed $old"
  exit 0
fi
rm -rf "$staged"
put_back "$dest" "$staged" || restore_failed
put_back "$old" "$dest" || restore_failed
rm -rf "$staged"
log "the new Deputy did not start within $launch_wait seconds; restored the previous version"
reopen
exit 1
