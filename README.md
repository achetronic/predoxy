# Predoxy

## Description

A proxy for Redis that converts all queries into transactions. It caches the last selected database and selects
it again on each request, to decrease risk on multi-tenant environments. Moreover, it can change or filter commands
on the fly, increasing the security.

## Use case

This application was initially designed to cover a very specific use case, having several instances of Redis, watched
by several instances of Sentinel (for passive replication between them). Moreover, in front of Redis machines,
you can see a HAproxy deployment, that is always forwarding traffic only to the machines that are up. In addition, 
our proxy is in front of everything, putting a bit of make up over the requests. 

This way, applications can be designed to connect to a single instance of Redis, but having a really powerful 
Redis deployment (HA, filters) backing up that horrible design pattern.

The diagram is as follows:

`TBD`

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
