#! /bin/bash

# us-central1 because us-east1 does not fucking have go111 runtime yet

# deploying func
gcloud functions deploy ComputeLiveAuctions \
    --runtime go111 \
    --trigger-topic computeLiveAuctions \
    --source . \
    --memory 256MB \
    --region us-central1 \
    --timeout 60s