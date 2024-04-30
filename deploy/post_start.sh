#!/usr/bin/env bash
# waiting for splunk to finish starting
# sleep 120
export SPLUNK_TOKEN=$(curl -k -u admin:splunkadmin -X POST https://splunk:8089/services/authorization/tokens?output_mode=json --data name=admin --data audience=splunk_exporter | jq -r '.entry[0].content.token')
cat splunk_exporter.yml.src | envsubst > splunk_exporter.yml
curl -X POST http://splunk_exporter:9115/-/reload