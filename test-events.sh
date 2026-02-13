#!/bin/bash
# Simple script to test event emission from boatmanmode
# Usage: ./test-events.sh

set -e

echo "🧪 Testing BoatmanMode Event Emission"
echo "======================================"
echo ""
echo "This script demonstrates the JSON event stream emitted by boatmanmode."
echo "Events are emitted to stdout and can be parsed by external tools."
echo ""
echo "Press Ctrl+C to stop at any time."
echo ""
echo "Starting in 3 seconds..."
sleep 3

# Use a simple prompt that will run quickly
PROMPT="Add a simple version endpoint at /version that returns {version: string}"

echo ""
echo "🚀 Running: boatman work --prompt \"$PROMPT\" --dry-run"
echo ""
echo "📡 Event Stream (JSON lines):"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Run boatman and filter for JSON events, pretty-print them
./boatman work --prompt "$PROMPT" --dry-run 2>&1 | while IFS= read -r line; do
    # Try to parse as JSON
    if echo "$line" | jq -e . >/dev/null 2>&1; then
        # It's valid JSON - pretty print it
        echo "📨 EVENT:"
        echo "$line" | jq '.'
        echo ""
    fi
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Event stream test complete!"
echo ""
echo "💡 To see all events from a real run:"
echo "   ./boatman work --prompt \"...\" | grep '^{' | jq"
echo ""
echo "💡 To count event types:"
echo "   ./boatman work --prompt \"...\" 2>&1 | grep '^{' | jq -r '.type' | sort | uniq -c"
