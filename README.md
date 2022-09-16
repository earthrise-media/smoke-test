# Smoke Test

A simple service that will execute HTTP requests on demand for the purposes of pre-deployment testings. Think [hey](https://github.com/rakyll/hey) but 
with more control over the request URLs.  

This was developed to run during [Flagger](https://github.com/fluxcd/flagger) blue/green deployments

## Usage 

Example: `http://localhost:8001/smoke-test?HOST=localhost&PORT=8123&PROTO=http&DURATION=10s&SERVICE=trace-asset-v0`

Params: 
- HOST: the hostname or IP where the target service is running
- PORT: the port of the target service
- PROTO: http/https
- DURATION: how long to run the tests: `1m` `90s` etc
- SERVICE: the name of the service --> translated into the name of the CSV file in your repo

## Configuration
The service uses the following env vars: 

- PORT: where to listen
- LOG_LEVEL: how verbose to log (default INFO)
- REPO_ROOT: where to look for service URL files. This doesn't need to be a git repo, just anywhere that will resolve the following pattern: `$REPO_ROOT+/+$SERVICE+".csv"`
- SLACK_CHANNEL: where to send start/end notifications
- SLACK_TOKEN: slack oauth token (starts with `xoxb`)