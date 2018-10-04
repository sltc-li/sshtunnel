# sshtunnel

A tool helps to do ssh forwarding.

## Features

* Usable as a CLI tool or as a library.

## Installation

To install the library and command line program, use the following:

```bash
go get -v github.com/liyy7/sshtunnel/...
```

## Usage

```bash
$ ssh-tunnel -help
Usage of ssh-tunnel:
  -c string
        JSON config file

$ ssh-tunnel -c config.json
```

See [config.json.sample](ssh-tunnel/config.json.sample) for format of config file.
