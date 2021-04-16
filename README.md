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
$ tunnel config.yml
```

See [config.yaml.sample](cmd/tunnel/config.yml.sample) for format of config file.

## Use go-bindata to build independent binary

```bash
$ git clone https://github.com/li-go/sshtunnel.git && cd sshtunnel
$ go get -u github.com/go-bindata/go-bindata
$ go-bindata -o=bindata.go -pkg=sshtunnel -tags=bindata ~/.ssh
$ go build -tags bindata ./cmd/tunnel/main.go
$ mv ./main ~/.go/bin/tunnel
```
