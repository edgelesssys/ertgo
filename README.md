# ertgo: Go compiler for EGo and Edgeless RT

This is a modified [Go compiler](https://github.com/golang/go) that is used in [EGo](https://github.com/edgelesssys/ego) and [Edgeless RT](https://github.com/edgelesssys/edgelessrt) to build enclaves written in Go.

## Build

This branch contains the ertgo runtime and prebuilt binaries.
It's ready to use.

It's created with the following steps:

1. The base is the binary release of the corresponding Go version, i.e., `go1.xx.x.linux-amd64.tar.gz`
2. Patches are applied to files under `src/` for compatibility with the enclave environment
3. `bin/go` is replaced by the executable built by executing `make.sh` in the `v1.xx-src` branch of this repository
