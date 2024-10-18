#!/bin/env bash

# exit if a command fail
set -e
# print commands
set -v

# Start the stack
export SPLUNK_IMAGE="splunk/splunk:9.2"
docker run --rm -it ${SPLUNK_IMAGE:-splunk/splunk:latest} create-defaults > default.yml
docker compose up -d --remove-orphans 

# Please wait for Splunk to be initialized, check this with the command:
# docker compose logs dmc -f
# If you need to reload config, you may use the following command:
# curl -X POST http://localhost:9115/-/reload
