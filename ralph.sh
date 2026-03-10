#!/bin/bash
# Ralph Wiggum - Long-running AI agent loop
# Usage: ./ralph.sh [max_iterations]

set -e

MAX_ITERATIONS="${1:-10}"

# Validate input is a positive integer
if ! [[ "$MAX_ITERATIONS" =~ ^[0-9]+$ ]] || [ "$MAX_ITERATIONS" -eq 0 ]; then
  echo "ERROR: max_iterations must be a positive integer, got: $MAX_ITERATIONS"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PRD_FILE="$SCRIPT_DIR/ralph/plan.json"
PROGRESS_FILE="$SCRIPT_DIR/ralph/PROGRESS.md"
ARCHIVE_DIR="$SCRIPT_DIR/ralph/archive"
LAST_BRANCH_FILE="$SCRIPT_DIR/ralph/.last-branch"
LOG_DIR="$SCRIPT_DIR/ralph/logs"

mkdir -p "$LOG_DIR"

# --- helpers ---

init_progress_file() {
  printf '%s\n' "# Ralph Progress Log" "Started: $(date)" "---" > "$PROGRESS_FILE"
}

get_prd_branch() {
  jq -r '.branchName // empty' "$PRD_FILE" 2>/dev/null || true
}

# --- signal handling ---

trap 'echo ""; echo "Interrupted at iteration ${i:-?} of $MAX_ITERATIONS"; exit 130' INT TERM

# --- archive previous run if branch changed ---

if [ -f "$PRD_FILE" ]; then
  CURRENT_BRANCH=$(get_prd_branch)

  if [ -f "$LAST_BRANCH_FILE" ]; then
    LAST_BRANCH=$(cat "$LAST_BRANCH_FILE" 2>/dev/null || true)

    if [ -n "$CURRENT_BRANCH" ] && [ -n "$LAST_BRANCH" ] && [ "$CURRENT_BRANCH" != "$LAST_BRANCH" ]; then
      FOLDER_NAME="${LAST_BRANCH#ralph/}"
      ARCHIVE_FOLDER="$ARCHIVE_DIR/$(date +%Y-%m-%d-%H%M%S)-$FOLDER_NAME"

      echo "Archiving previous run: $LAST_BRANCH"
      mkdir -p "$ARCHIVE_FOLDER"
      cp "$PRD_FILE" "$ARCHIVE_FOLDER/"
      [ -f "$PROGRESS_FILE" ] && cp "$PROGRESS_FILE" "$ARCHIVE_FOLDER/"
      echo "   Archived to: $ARCHIVE_FOLDER"

      init_progress_file
    fi
  fi

  # Track current branch
  if [ -n "$CURRENT_BRANCH" ]; then
    printf '%s\n' "$CURRENT_BRANCH" > "$LAST_BRANCH_FILE"
  fi
fi

# Initialize progress file if it doesn't exist
[ -f "$PROGRESS_FILE" ] || init_progress_file

# --- main loop ---

CONSECUTIVE_BLOCKED=0
MAX_CONSECUTIVE_BLOCKED=2

echo "Starting Ralph - Max iterations: $MAX_ITERATIONS"

for i in $(seq 1 "$MAX_ITERATIONS"); do
  echo ""
  echo "==============================================================="
  echo "  Ralph Iteration $i of $MAX_ITERATIONS"
  echo "==============================================================="

  LOG_FILE="$LOG_DIR/iteration-$i.log"

  # Run claude, tee to stderr for live output, write log directly
  CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=99 claude --dangerously-skip-permissions --print \
    < "$SCRIPT_DIR/ralph/RALPH.md" 2>&1 \
    | tee /dev/stderr > "$LOG_FILE" \
    || true

  # Parse promise tag from the log file (single grep pass)
  if grep -q '<promise>COMPLETE</promise>' "$LOG_FILE"; then
    echo ""
    echo "Ralph completed all tasks!"
    echo "Completed at iteration $i of $MAX_ITERATIONS"
    exit 0
  elif grep -q '<promise>BLOCKED</promise>' "$LOG_FILE"; then
    CONSECUTIVE_BLOCKED=$((CONSECUTIVE_BLOCKED + 1))
    echo "WARNING: Task blocked ($CONSECUTIVE_BLOCKED consecutive)"
    if [ "$CONSECUTIVE_BLOCKED" -ge "$MAX_CONSECUTIVE_BLOCKED" ]; then
      echo "ERROR: $MAX_CONSECUTIVE_BLOCKED consecutive blocked iterations. Aborting."
      echo "See logs: $LOG_DIR/"
      exit 1
    fi
    sleep 2
    continue
  elif grep -q '<promise>ITERATION_DONE</promise>' "$LOG_FILE"; then
    echo "Task completed. Moving to next iteration..."
    CONSECUTIVE_BLOCKED=0
  else
    echo "ERROR: No promise tag detected — agent violated the one-task protocol. Aborting."
    echo "See log: $LOG_FILE"
    exit 1
  fi
done

echo ""
echo "Ralph reached max iterations ($MAX_ITERATIONS) without completing all tasks."
echo "Check $PROGRESS_FILE for status."
exit 1
