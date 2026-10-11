#!/bin/bash
# NeXuS ChaCha Watcher — autonomous Claude orchestrator
# Monitors CHACHA.md for Gemini updates, runs builds, fixes errors, drives phases

CHACHA="/home/user/claude/nexus-orchestrator/CHACHA.md"
PROJECT="/home/user/claude/char-gen-cli"
GEMINI_PANE="3_restored_022943:0.1"
LOG="/tmp/nexus_watch.log"
LAST_LINE_FILE="/tmp/nexus_watch_lastline"

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a "$LOG"; }

backup_file() {
    local f="$PROJECT/$1"
    if [ -f "$f" ]; then
        local n=1
        while [ -f "${f}.bak${n}" ]; do n=$((n+1)); done
        cp "$f" "${f}.bak${n}"
        log "BACKUP: $1 → $1.bak${n}"
    fi
}

post_to_chacha() {
    cat >> "$CHACHA" << ENTRY

---
[CLAUDE] $(date '+%Y-%m-%d %H:%M')
TOPIC: $1

$2

---
ENTRY
    log "CHACHA: posted '$1'"
}

tell_gemini() {
    tmux send-keys -t "$GEMINI_PANE" "$1" Enter
    log "→ GEMINI: $1"
}

run_build() {
    log "BUILD: running go build ./..."
    cd "$PROJECT"
    BUILD_OUT=$(go build ./... 2>&1)
    BUILD_EXIT=$?
    if [ $BUILD_EXIT -eq 0 ]; then
        log "BUILD: ✅ PASSED"
        post_to_chacha "Build Verification" "go build ./... ✅ PASSED — clean build. Proceed to next step."
        tell_gemini "Build passed ✅ Read CHACHA for next instructions."
        return 0
    else
        log "BUILD: ❌ FAILED"
        log "$BUILD_OUT"
        post_to_chacha "Build FAILED — errors below" "go build ./... ❌ FAILED

\`\`\`
$BUILD_OUT
\`\`\`

Fix the errors above. Do NOT touch any other files. Post to CHACHA when fixed."
        tell_gemini "Build FAILED ❌ — Read CHACHA for errors. Fix and post when done."
        return 1
    fi
}

# Track last seen line count in CHACHA
[ -f "$LAST_LINE_FILE" ] && LAST_LINES=$(cat "$LAST_LINE_FILE") || LAST_LINES=$(wc -l < "$CHACHA")
echo "$LAST_LINES" > "$LAST_LINE_FILE"

log "=== NeXuS ChaCha Watcher started ==="
log "Monitoring: $CHACHA"
log "Project: $PROJECT"

while true; do
    CURRENT_LINES=$(wc -l < "$CHACHA")
    
    if [ "$CURRENT_LINES" -gt "$LAST_LINES" ]; then
        # New content detected
        NEW_CONTENT=$(tail -n $((CURRENT_LINES - LAST_LINES)) "$CHACHA")
        echo "$CURRENT_LINES" > "$LAST_LINE_FILE"
        LAST_LINES=$CURRENT_LINES
        
        log "CHACHA updated (+$((CURRENT_LINES - LAST_LINES + CURRENT_LINES - LAST_LINES)) lines)"
        
        # Check if Gemini posted
        if echo "$NEW_CONTENT" | grep -q "^\[GEMINI\]"; then
            log "Gemini posted — analyzing..."
            
            # Check for backup requests
            if echo "$NEW_CONTENT" | grep -qi "READY FOR BACKUP"; then
                FILES=$(echo "$NEW_CONTENT" | grep -i "READY FOR BACKUP" | grep -oP '[\w.]+\.go' | head -5)
                for f in $FILES; do
                    backup_file "$f"
                done
                post_to_chacha "Backups Complete" "All requested backups done. Proceed with edits."
                tell_gemini "Backups done ✅ Proceed with your edits. Post to CHACHA when complete."
                
            # Check for build request
            elif echo "$NEW_CONTENT" | grep -qi "BUILD\|verify\|done\|complete\|finished"; then
                sleep 2  # brief pause for file writes to settle
                run_build
                
            # Check for phase completion
            elif echo "$NEW_CONTENT" | grep -qi "PHASE.*COMPLETE\|phase.*done"; then
                run_build
            fi
        fi
    fi
    
    sleep 15
done
