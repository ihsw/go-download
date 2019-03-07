#! /bin/bash

# us-central1 because us-east1 does not fucking have go111 runtime yet

# deploying func
gcloud functions deploy PricelistHistoriesComputeIntake \
    --runtime go111 \
    --trigger-topic pricelistHistoriesCompute \
    --source . \
    --region us-central1 \
    --memory 512MB \
    --timeout 120s