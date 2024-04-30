#!/bin/env bash

# exit if a command fail
set -e
# print commands
set -v


# Start the stack
docker compose up -d

# Wait for splunk to be initialized
until docker logs -n1 splunk 2>/dev/null | grep -q -m 1 '^Ansible playbook complete'; do sleep 0.2; done

# Generate api key
post_start_image=$(docker build -q - < Dockerfile-post_start)
touch ./splunk_exporter.yml
docker run \
    --rm \
    --volume ./post_start.sh:/post_start.sh:ro \
    --volume ./splunk_exporter.yml:/splunk_exporter.yml:rw \
    --volume ./splunk_exporter.yml.src:/splunk_exporter.yml.src:ro \
    --entrypoint bash \
    --entrypoint /post_start.sh \
    --network deploy_monitoring \
    $post_start_image
    
