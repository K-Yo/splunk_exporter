#!/bin/env bash

# exit if a command fail
set -e
# print commands
set -v

# initiate conf file
touch ./splunk_exporter.yml

# Start the stack
docker compose up -d prometheus grafana splunk

# Wait for splunk to be initialized
until docker logs -n1 splunk 2>/dev/null | grep -q -m 1 '^Ansible playbook complete'; do sleep 0.2; done

# Generate api key
export SPLUNK_TOKEN=$(curl -k -u admin:splunkadmin -X POST https://localhost:8089/services/authorization/tokens?output_mode=json --data name=admin --data audience=splunk_exporter | jq -r '.entry[0].content.token')
cat splunk_exporter.yml.src | envsubst > splunk_exporter.yml

# start splunk_exporter
docker compose up -d
# curl -X POST http://localhost:9115/-/reload
