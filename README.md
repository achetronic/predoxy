# Predoxy

## Description

A hackable TCP proxy where the user decide what to do with the incoming message and the outgoing response, 
implementing plugins as Go functions

The proxy provides some useful data and memory space to each plugin execution to:

- Know information about the source/destination connection
- Store data on a common space of memory
- Read/Write the message on the fly

## Use case

This is complicated to be defined, but let me give you some ideas about what to do with the plugins:

- Filter/change messages according to for regex patterns. This allows to do things like neutralizing commands on 
  Redis like EVAL, or converting them in something more creative, like ECHO clauses
- Use the InMemory cache to store data from previous requests coming from a source. These data can be used to inject
  or parse the message in following requests
- Enforce policies not covered by servers like Redis, i.e, forcing AUTH for SELECTed DBs, caching the requests 
  into the cache

## Requirements

- [Golang](https://go.dev/dl/) `1.18+`
- [Docker DE](https://docs.docker.com/get-docker/) `20.10+`
- [Kubectl](https://kubernetes.io/docs/tasks/tools/) `1.24+` (Optional, only if you want to deploy this on Kubernetes)

## Build

> All the commands must be executed from the root path of the repository

1. The first step is to test the code to catch the bugs in advance, so better to execute the following.

    ```bash
    make test
    ```

2. Compile the code. This will create a binary called `predoxy` inside the `bin/` directory

    ```bash
    make build
    ```
   
   > If you need to execute the code without compiling it, just execute `make run`

3. The next thing you need is to build the Docker image, to be able to deploy the project inside Kubernetes

    ```bash
    export IMG=predoxy:latest
    make docker-build
    make docker-push
    ```
   
## How to build a plugin
TBD

## Flags

There are several flags that can be configured to change the behaviour of the
application. They are described in the following table:

| Name              | Description                    |    Default    | Example          |
|:------------------|:-------------------------------|:-------------:|:-----------------|
| `--zap-log-level` | Log level                      |    `info`     | `debug`, `error` |
| `--config`        | Path to the `config.yaml` file | `config.yaml` | -                |

## Examples

To execute the process on your machine, execute the command as follows:

```sh
predoxy --zap-log-level debug \
        --config "my/path/config.yaml" 
```

To run the same into a Docker container:

```bash
docker run TBD
```

## How to contribute

We are open to external collaborations for this project: improvements, bugfixes, whatever.

For doing it, open an issue to discuss the need of the changes, then:

- Fork the repository
- Make your changes to the code
- Open a PR and wait for review

The code will be reviewed and tested (always)

> We are developers and hate bad code. For that reason we ask you the highest quality
> on each line of code to improve this project on each iteration.
