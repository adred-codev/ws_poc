#!/bin/bash
# Automated backend setup - fully idempotent and secure
# Usage: ./auto-setup-backend.sh <instance> <zone> <project> <branch> [git_pat]
# Note: git_pat is optional - only needed for private repos

set -e

INSTANCE=${1:-odin-backend}
ZONE=${2:-us-central1-a}
PROJECT=${3:-odin-ws-server}
GIT_BRANCH=${4:-working-refactored-12k}
GIT_PAT=${5:-}

echo "🤖 Automated Backend Setup"
echo "   Instance: $INSTANCE"
echo "   Zone: $ZONE"
echo "   Branch: $GIT_BRANCH"
echo ""

# Build clone URL securely (NEVER echo this variable)
if [ -n "$GIT_PAT" ]; then
    CLONE_URL="https://${GIT_PAT}@github.com/adred-codev/ws_poc.git"
    echo "🔐 Using authenticated clone"
else
    CLONE_URL="https://github.com/adred-codev/ws_poc.git"
    echo "🌐 Using public clone"
fi

# Check if repo exists
echo "🔍 Checking repository state..."
REPO_EXISTS=$(gcloud compute ssh $INSTANCE \
    --zone=$ZONE \
    --project=$PROJECT \
    --command="sudo -u deploy test -d /home/deploy/ws_poc && echo 'true' || echo 'false'" \
    2>/dev/null || echo "false")

if [ "$REPO_EXISTS" = "true" ]; then
    echo "✅ Repository exists, pulling latest..."
    # Pull without showing remote URL
    gcloud compute ssh $INSTANCE \
        --zone=$ZONE \
        --project=$PROJECT \
        --command="sudo -u deploy bash -c 'cd ws_poc && git fetch origin $GIT_BRANCH 2>&1 | grep -v \"^From\" && git reset --hard origin/$GIT_BRANCH 2>&1 | grep -v \"^HEAD\"'" \
        2>/dev/null || true
    echo "✅ Code updated"
else
    echo "📦 Cloning repository..."
    # Clone without showing output (contains URL/PAT)
    gcloud compute ssh $INSTANCE \
        --zone=$ZONE \
        --project=$PROJECT \
        --command="sudo -u deploy git clone -b $GIT_BRANCH -q $CLONE_URL /home/deploy/ws_poc" \
        2>&1 | grep -v "Cloning" | grep -v "remote" || true
    echo "✅ Repository cloned"
fi

# Start Docker services
echo "🐳 Starting Docker services..."
gcloud compute ssh $INSTANCE \
    --zone=$ZONE \
    --project=$PROJECT \
    --command="sudo -u deploy bash -c 'cd /home/deploy/ws_poc/deployments/v1/gcp/distributed/backend && docker compose up -d'" \
    2>&1 | grep -E "(Creating|Starting|Started|Created|Running)" || true

echo "⏳ Waiting for services to initialize (15s)..."
sleep 15

# Check if topics exist
echo "🔍 Checking Kafka topics..."
TOPICS_EXIST=$(gcloud compute ssh $INSTANCE \
    --zone=$ZONE \
    --project=$PROJECT \
    --command="docker exec redpanda rpk topic list 2>/dev/null | grep -q 'odin.trades' && echo 'true' || echo 'false'" \
    2>/dev/null || echo "false")

if [ "$TOPICS_EXIST" = "false" ]; then
    echo "📋 Creating Kafka topics..."
    gcloud compute ssh $INSTANCE \
        --zone=$ZONE \
        --project=$PROJECT \
        --command="docker exec redpanda rpk topic create odin.trades odin.liquidity odin.balances odin.metadata odin.social odin.community odin.creation odin.analytics --partitions 12 --replicas 1" \
        2>/dev/null | grep "TOPIC" || true
    echo "✅ Topics created"
else
    echo "✅ Topics already exist"
fi

# Verify services
echo "🔍 Verifying services..."
CONTAINERS=$(gcloud compute ssh $INSTANCE \
    --zone=$ZONE \
    --project=$PROJECT \
    --command="docker ps --format '{{.Names}}' | wc -l" \
    2>/dev/null || echo "0")

echo "   Running containers: $CONTAINERS"

echo ""
echo "✅ Backend setup complete"
