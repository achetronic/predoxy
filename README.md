# Predoxy

## Description

A hackable TCP proxy where the user decide what to do with the incoming message and the outgoing response, 
implementing plugins as Go functions

The proxy injects useful parameters for each plugin execution to:

- Know information about the source/destination connection
- Cache data on an individual space of memory
- Read/write the message on the fly

## Use case

This is complicated to be defined, but let me give you some ideas about what to do with the plugins:

- Filter/change messages according to regex patterns. This allows to do things like neutralizing commands on 
  Redis like EVAL, or converting them in something more creative, like ECHO clauses, or even translate a whole protocol
  into another
- Use the InMemory cache to store data from previous requests coming from a source. These data can be used to inject
  or parse the message in following requests
- Enforce policies not covered by servers like Redis, i.e, forcing AUTH for SELECTed DBs, caching the requests 
  into the cache
- Debug your own coded proxies logging all the messages in the middle

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

All the plugins must implement the `Plugin` interface, which can be imported from `github.com/achetronic/predoxy/api`

Examples are far better than explanations, so you have the implementation of `do-nothing` in the following lines:

```go 
package main

import (
	"github.com/achetronic/predoxy/api"
	"log"
)

type customPlugin string

// OnReceive TODO
func (p *customPlugin) OnReceive(parameters *api.PluginParams) error {

	log.Print("Basically, I'm not doing anything on receive")
	log.Print(string(*(*parameters).Message))
	return nil
}

// OnResponse TODO
func (p *customPlugin) OnResponse(parameters *api.PluginParams) error {

	log.Print("Basically, I'm not doing anything on response")
	log.Print(string(*(*parameters).Message))
	return nil
}

var Plugin customPlugin
```

> The previous example can be found on [examples](examples) directory and probably some others could be added in the future.

As you can see, `*api.PluginParams` is always injected, with some useful things that can be used to craft 
meaningful plugins:

```go
type PluginParams struct {
	SourceConnection *net.Conn
	DestConnection   *net.Conn
	Message          *[]byte
	LocalCache       *bigcache.BigCache // Ref: https://github.com/allegro/bigcache
}
```

> **ATTENTION:** This proxy is created using a lof of pointers to increase the performance. Those pointers mean that
> you have to code carefully your plugins to decrease the attack surface your own. I recommend you to use only
> plugins built from sources you trust.

Once you have your plugin ready to rock, compile it into a `.so` executing the following from the root path of it:

```console
go build --buildmode=plugin
```

If everything worked fine, this will generate the .so file. Just set the path to your `.so` files on the `config.yaml`
and everything will be ready.

## How to configure

A complete example of configuration, can be found on [examples](examples) directory

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

## How to contribute

We are open to external collaborations for this project: improvements, bugfixes, plugins examples, whatever.

For doing it, open an issue to discuss the changes, then:

- Fork the repository
- Make your changes to the code
- Open a PR and wait for review

The code will be reviewed and tested (always)
