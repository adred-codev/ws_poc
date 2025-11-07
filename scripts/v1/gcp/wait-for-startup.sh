#!/bin/bash
# Wait for startup script to complete on GCP instance
# Usage: ./wait-for-startup.sh <instance> <zone> <project> [timeout]

set -e

INSTANCE=$1
ZONE=$2
PROJECT=$3
TIMEOUT=${4:-300}

if [ -z "$INSTANCE" ] || [ -z "$ZONE" ] || [ -z "$PROJECT" ]; then
    echo "Usage: $0 <instance> <zone> <project> [timeout]"
    exit 1
fi

echo "⏳ Waiting for $INSTANCE startup script to complete..."
ELAPSED=0
INTERVAL=10

while [ $ELAPSED -lt $TIMEOUT ]; do
    if gcloud compute ssh $INSTANCE \
        --zone=$ZONE \
        --project=$PROJECT \
        --command="test -f /var/log/startup-complete.log && echo 'COMPLETE' || echo 'PENDING'" \
        --ssh-flag="-o ConnectTimeout=5" \
        --ssh-flag="-o StrictHostKeyChecking=no" \
        2>/dev/null | grep -q "COMPLETE"; then
        echo "✅ Startup script complete (${ELAPSED}s)"
        exit 0
    fi

    echo "   Waiting... (${ELAPSED}s/${TIMEOUT}s)"
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done

echo "⚠️  Timeout after ${TIMEOUT}s - instance may still be initializing"
exit 0  # Don't fail - just warn
