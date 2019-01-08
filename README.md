# sshtunnel

A tool helps to do ssh forwarding.

## Features

* Usable as a CLI tool or as a library.

## Installation

To install the library and command line program, use the following:

```bash
go get -u github.com/liyy7/sshtunnel/tunnel
```

## Usage

```bash
$ tunnel config.json
```

See [config.json.sample](tunnel/config.json.sample) for format of config file.

## Use go-bindata to build independent binary

```bash
$ git clone https://github.com/liyy7/sshtunnel.git && cd sshtunnel
$ go get -u github.com/go-bindata/go-bindata
$ go-bindata -o=bindata.go -pkg=sshtunnel -tags=bindata ~/.ssh
$ go build -tags bindata tunnel/main.go
$ mv ./main ~/.go/bin/tunnel
```
