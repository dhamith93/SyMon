# Agent 
Collects metrics and send out to collector

# Collector
`./collector --init` creates a key
`./collector --init-agent <server-id>` creates db for agent

## DB
SQLite
Seperate DBs for each agent server_id.db

# Client
Client for users
web client 

## AGENT ENV VARS
SYMON_SERVER_ID