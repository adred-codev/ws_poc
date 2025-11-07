#!/bin/bash
# Wait for SSH to be ready on GCP instance
# Usage: ./wait-for-ssh.sh <instance> <zone> <project> [timeout]

set -e

INSTANCE=$1
ZONE=$2
PROJECT=$3
TIMEOUT=${4:-300}

if [ -z "$INSTANCE" ] || [ -z "$ZONE" ] || [ -z "$PROJECT" ]; then
    echo "Usage: $0 <instance> <zone> <project> [timeout]"
    exit 1
fi

echo "⏳ Waiting for $INSTANCE SSH to be ready..."
ELAPSED=0
INTERVAL=10

while [ $ELAPSED -lt $TIMEOUT ]; do
    if gcloud compute ssh $INSTANCE \
        --zone=$ZONE \
        --project=$PROJECT \
        --command="echo 'ready'" \
        --ssh-flag="-o ConnectTimeout=5" \
        --ssh-flag="-o StrictHostKeyChecking=no" \
        2>/dev/null | grep -q "ready"; then
        echo "✅ SSH ready (${ELAPSED}s)"
        exit 0
    fi

    echo "   Waiting... (${ELAPSED}s/${TIMEOUT}s)"
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done

echo "❌ Timeout waiting for SSH after ${TIMEOUT}s"
exit 1
