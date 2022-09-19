# Smoke Test

[![Go Report Card](https://goreportcard.com/badge/github.com/earthrise-media/smoke-test)](https://goreportcard.com/report/github.com/earthrise-media/smoke-test)

[![Build](https://github.com/earthrise-media/smoke-test/actions/workflows/build.yaml/badge.svg?branch=main)](https://github.com/earthrise-media/smoke-test/actions/workflows/build.yaml)

A simple service that will execute HTTP requests on demand for the purposes of pre-deployment testings. Think [hey](https://github.com/rakyll/hey) but 
with remote request URLs, slack and a few other tweaks.  

This was developed to run during [Flagger](https://github.com/fluxcd/flagger) blue/green deployments updates.

## Usage 

Example: `http://localhost:8001/smoke-test?HOST=localhost&PORT=8123&PROTO=http&DURATION=10s&SERVICE=trace-asset-v0`

Params: 
- HOST: the hostname or IP where the target service is running
- PORT: the port of the target service
- PROTO: http/https
- DURATION: how long to run the tests: `1m` `90s` etc
- SERVICE: the name of the service --> translated into the name of the CSV file in your repo

Sample canary that uses this:

```
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: api-canary
  namespace: api
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-deployment
  service:
    port: 8124
  #this is greatly reduced for dev
  analysis:
    interval: 10s
    threshold: 3
    maxWeight: 40
    stepWeight: 10
    metrics:
      - name: request-success-rate
        thresholdRange:
          min: 99
        interval: 1m
      - name: request-duration
        thresholdRange:
          max: 500
        interval: 30s
    webhooks:
      - name: load-test
        type: rollout
        url: http://load-generator.api.svc.cluster.local:8001/smoke-test?HOST=asset-api-deployment-canary.climatetrace.svc.cluster.local&PORT=8124&PROTO=http&DURATION=1m&SERVICE=asset-ap
i-v0
```


## Configuration
The service uses the following env vars: 

- PORT: where to listen
- LOG_LEVEL: how verbose to log (default INFO)
- REPO_ROOT: where to look for service URL files. This doesn't need to be a git repo, just anywhere that will resolve the following pattern: `$REPO_ROOT+/+$SERVICE+".csv"`
- SLACK_CHANNEL: where to send start/end notifications
- SLACK_TOKEN: slack oauth token (starts with `xoxb`)
