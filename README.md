# Imgcat

[![Go Report Card](https://goreportcard.com/badge/github.com/danielgatis/imgcat?style=flat-square)](https://goreportcard.com/report/github.com/danielgatis/imgcat)
[![License MIT](https://img.shields.io/badge/license-MIT-blue.svg)](https://raw.githubusercontent.com/danielgatis/imgcat/master/LICENSE)
[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg?style=flat-square)](https://godoc.org/github.com/danielgatis/imgcat)
[![Release](https://img.shields.io/github/release/danielgatis/imgcat.svg?style=flat-square)](https://github.com/danielgatis/imgcat/releases/latest)

Display images and gifs in your terminal emulator.

![demo](demo.gif)

### Installation

First, [install Go](https://golang.org/doc/install).

Next, fetch and build the binary.

```bash
go get -u github.com/danielgatis/imgcat
```

Alternatively, you can download a pre-built binary [here](https://github.com/danielgatis/imgcat/releases).

### Usage

Display a remote image

```
curl -s http://input.png | imgcat
```

Display a local image

```
imgcat path/to/image.png
```

### Requirements

Your terminal emulator must be support `true color` and use a `monospaced font` that includes the lower half block unicode character (`▄ U+2584`).

### License

Copyright (c) 2020-present [Daniel Gatis](https://github.com/danielgatis)

Licensed under [MIT License](./LICENSE)
