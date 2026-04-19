#!/usr/bin/env bash

set -e

ITERATIONS=${1:-80}

# Validate prerequisites
if [ ! -f "TASKS.md" ]; then
    echo "Error: @TASKS.md not found in current directory"
    exit 1
fi

# Detect project type and set appropriate commands
detect_project_type() {
    if [ -f "composer.json" ]; then
        echo "php"
    elif [ -f "app.json" ] && grep -q '"expo"' app.json 2>/dev/null; then
        echo "expo"
    elif [ -f "package.json" ]; then
        echo "nodejs"
    elif [ -f "pyproject.toml" ] || [ -f "requirements.txt" ]; then
        echo "python"
    else
        echo "generic"
    fi
}

PROJECT_TYPE=$(detect_project_type)

# Cleanup function
cleanup() {
    rm -f .ralph-complete
}
trap cleanup EXIT

echo "Starting Ralph Wiggum method with $ITERATIONS iterations..."
echo "Detected project type: $PROJECT_TYPE"

for ((i = 1; i <= ITERATIONS; i++)); do
    echo "[Iteration $i/$ITERATIONS] Running task automation..."

    if ! claude --dangerously-skip-permissions --model=opus "$(cat <<'EOF'
1. Read `@TASKS.md` file.
2. Find the first incomplete task (first unchecked `[ ]` item, scanning top to bottom) and mark it as `[ONGOING]` before starting. Do not skip tasks or pick freely — always pick the first available one.
3. Implement the task. Include tests where practical.
4. Run formatting/linting to ensure code quality:
   - For Go: gofmt -w . && go vet ./...
   - For React/Expo: npm run format (Prettier) or npx prettier --write .
   - For PHP: composer run format:dirty
   - For Python: black . or similar
5. Commit your changes.
6. Update `@TASKS.md`: mark the task as `[x]` and summarize what you did in a "Progress" section at the bottom.
7. If there are no remaining incomplete tasks, create a file named `.ralph-complete` containing the text "done".

IMPORTANT: Read @../../sites/cluade-code, this is the ai agent i am trying to copy!
EOF
)"; then
        echo "Error: Claude command failed on iteration $i"
        exit 1
    fi

    if [ -f .ralph-complete ]; then
        echo "✓ All tasks are now complete! It took $i iterations."
        exit 0
    fi
done

echo "Warning: Reached iteration limit ($ITERATIONS) without completing all tasks."
exit 1
