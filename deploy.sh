#!/usr/bin/env bash

set -euo pipefail

usage(){
    echo "usage ${0##*/} AWS_REGION PROMETHEUS_WORKSPACE_ID"
    echo "> Ensure Function roles are set as environment variables"
    echo "> METRIC_NAMES_FUNCTION_ROLE_ARN, COUNT_METRICS_FUNCTION_ROLE_ARN, AGGREGATE_COUNT_FUNCTION_ROLE_ARN, INGEST_AMP_FUNCTION_ROLE_ARN"
    return 1
}

[ $# != 2 ] && usage

command -v aws >/dev/null 2>&1 ||
    { echo >&2 "ERR: awscli is missing, aborting!"; exit 1; }

command -v sam >/dev/null 2>&1 ||
    { echo >&2 "ERR: aws-sam is missing, aborting!"; exit 1; }

# checking roles environment variables are set
[ -z "${METRIC_NAMES_FUNCTION_ROLE_ARN}" ] && { echo >&2 "ERR: METRIC_NAMES_FUNCTION_ROLE_ARN is missing, aborting!"; exit 1; }
[ -z "${COUNT_METRICS_FUNCTION_ROLE_ARN}" ] && { echo >&2 "ERR: COUNT_METRICS_FUNCTION_ROLE_ARN is missing, aborting!"; exit 1; }
[ -z "${AGGREGATE_COUNT_FUNCTION_ROLE_ARN}" ] && { echo >&2 "ERR: AGGREGATE_COUNT_FUNCTION_ROLE_ARN is missing, aborting!"; exit 1; }
[ -z "${INGEST_AMP_FUNCTION_ROLE_ARN}" ] && { echo >&2 "ERR: INGEST_AMP_FUNCTION_ROLE_ARN is missing, aborting!"; exit 1; }


AWS_REGION=$1
PROMETHEUS_WORKSPACE_ID=$2

echo "Deploying to AWS region ${AWS_REGION} with Prometheus workspace ID ${PROMETHEUS_WORKSPACE_ID}"

PROMETHEUS_REMOTE_WRITE_URL=$(aws --region ${AWS_REGION} amp describe-workspace \
    --workspace-id ${PROMETHEUS_WORKSPACE_ID} \
    --query "workspace.prometheusEndpoint" \
    --output text) || { echo >&2 "ERR: Failed to get Prometheus remote write URL, aborting!"; exit 1; }

PROMETHEUS_REMOTE_WRITE_URL="${PROMETHEUS_REMOTE_WRITE_URL}api/v1/remote_write"

sed -e "s/AMP_REGION/${AWS_REGION}/g" -e "s~AMP_REMOTE_WRITE_ENDPOINT~${PROMETHEUS_REMOTE_WRITE_URL}~g" lambda/ingest-amp/collector.yaml.tpl > lambda/ingest-amp/collector.yaml

cd lambda

sam build
sam sync --stack-name amp-ingest-insights \
    --parameter-overrides WorkspaceId=${PROMETHEUS_WORKSPACE_ID} \
    --parameter-overrides MetricNamesFunctionRoleARN=${METRIC_NAMES_FUNCTION_ROLE_ARN} \
    --parameter-overrides CountMetricsFunctionRoleARN=${COUNT_METRICS_FUNCTION_ROLE_ARN} \
    --parameter-overrides AggregateCountFunctionRoleARN=${AGGREGATE_COUNT_FUNCTION_ROLE_ARN} \
    --parameter-overrides IngestAMPFunctionRoleARN=${INGEST_AMP_FUNCTION_ROLE_ARN} \
    --no-watch
cd -
