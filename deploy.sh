#!/usr/bin/env bash

set -euo pipefail

usage(){
    echo "usage ${0##*/} AWS_REGION PROMETHEUS_WORKSPACE_ID [CFN-stack-name]"
    return 1
}

[ $# != 2 ] && usage

command -v aws >/dev/null 2>&1 ||
    { echo >&2 "ERR: awscli is missing, aborting!"; exit 1; }

command -v sam >/dev/null 2>&1 ||
    { echo >&2 "ERR: aws-sam is missing, aborting!"; exit 1; }

AWS_REGION=$1
PROMETHEUS_WORKSPACE_ID=$2
STACK_NAME="${3:-'amp-cardinality-insights'}"

echo "Deploying to AWS region ${AWS_REGION} with Prometheus workspace ID ${PROMETHEUS_WORKSPACE_ID}"

PROMETHEUS_REMOTE_WRITE_URL=$(aws --region ${AWS_REGION} amp describe-workspace \
    --workspace-id ${PROMETHEUS_WORKSPACE_ID} \
    --query "workspace.prometheusEndpoint" \
    --output text) || { echo >&2 "ERR: Failed to get Prometheus remote write URL, aborting!"; exit 1; }

PROMETHEUS_REMOTE_WRITE_URL="${PROMETHEUS_REMOTE_WRITE_URL}api/v1/remote_write"

sed -e "s/AMP_REGION/${AWS_REGION}/g" -e "s~AMP_REMOTE_WRITE_ENDPOINT~${PROMETHEUS_REMOTE_WRITE_URL}~g" lambda/ingest-amp/collector.yaml.tpl > lambda/ingest-amp/collector.yaml

cd lambda

sam build
sam sync --stack-name $STACK_NAME \
    --parameter-overrides WorkspaceId=${PROMETHEUS_WORKSPACE_ID} \
    --no-watch
cd -
