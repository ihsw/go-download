#! /bin/bash

# us-central1 because us-east1 does not fucking have go111 runtime yet

# deploying func
gcloud functions deploy AuctionsCollector \
    --runtime go111 \
    --trigger-topic auctionsCollectorCompute \
    --source . \
    --memory 128MB \
    --timeout 300s \
    --region us-central1