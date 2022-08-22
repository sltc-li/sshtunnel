# sshtunnel

[![Actions Status](https://github.com/li-go/sshtunnel/workflows/Go/badge.svg)](https://github.com/li-go/sshtunnel/actions)

A tool helps to do ssh forwarding.

## Features

* Usable as a CLI tool or as a library.

## Installation

To install the library and command line program, use the following:

```bash
$ go install github.com/li-go/sshtunnel/cmd/tunnel@latest
```

## Usage

```bash
$ tunnel -h
NAME:
   tunnel - a tool helps to do ssh forwarding

USAGE:
   tunnel [global options] command [command options] [arguments...]

VERSION:
   0.9.0

COMMANDS:
   status   show daemon process status
   kill     kill daemon process
   logs     show daemon process logs
   reload   reload config
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --config value, -c value  specify a yaml config file (default: "./.tunnel.yml")
   --daemon, -d              daemonize tunnel (default: false)
   --pidfile value           specify pid file for daemon process (default: "./.tunnel.pid")
   --logfile value           specify log file for daemon process (default: "./.tunnel.log")
   --help, -h                show help (default: false)
   --version, -v             print the version (default: false)
```

See [config.yaml.sample](cmd/tunnel/config.yml.sample) for format of config file.

## Configuration File
`tunnel` by default consults a few locations for the config files.

1. specified in `--config` or `-c`
2. `./.tunnel.yml`
3. `$XDG_CONFIG_HOME/sshtunnel/.tunnel.yml`
4. `$HOME/.tunnel.yml`

## Use go-bindata to build independent binary

```bash
$ git clone https://github.com/li-go/sshtunnel.git && cd sshtunnel
$ go get -u github.com/go-bindata/go-bindata
$ go-bindata -o=bindata.go -pkg=sshtunnel -tags=bindata ~/.ssh
$ go build -tags bindata ./cmd/tunnel/main.go
$ mv ./main ~/.go/bin/tunnel
```
