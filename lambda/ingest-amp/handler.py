import json
import time
from opentelemetry.metrics import CallbackOptions, Observation
from opentelemetry import metrics
from typing import Iterable

meter = metrics.get_meter("amp_ingest")


class SimpleGauge:
    def __init__(self, name, values, description):
        self.name = name
        self.description = description
        self.values = values
        self.observations = []

        def emit_observations(options: CallbackOptions) -> Iterable[Observation]:
            for v in values:
                yield Observation(v["value"], v["attributes"])

        meter.create_observable_gauge(
            name=self.name,
            description=self.description,
            callbacks=[emit_observations])


def lambda_handler(event, context):
    if not event:
        return {
            'statusCode': 404,
            'body': json.dumps('No event provided')
        }

    values = []

    for record in event['Records']:
        # Get the payload from the event
        print(record['body'])
        payload = json.loads(record['body'])
        values.append({
            "value": int(payload['count']),
            "attributes": {'metric_name': payload['name']}
        })

    metrics_cardinality = SimpleGauge(
        "metrics_cardinality_count",
        values,
        "Cardinality count for top N metrics in the workspace",
    )

    time.sleep(2)

    return {
        'statusCode': 200,
        'body': json.dumps('Processed records')
    }
