[![CI](https://github.com/spacescale/core/actions/workflows/pipeline.yaml/badge.svg)](https://github.com/spacescale/core/actions/workflows/pipeline.yaml)
[![Go](https://img.shields.io/badge/go-1.26.4-00ADD8)](https://go.dev/doc/go1.26)
[![License](https://img.shields.io/github/license/spacescale/core)](./LICENSE)

# core

This repository contains the SpaceScale platform runtime. The control plane,edge execution layer, and infrastructure tooling.

## Development

```bash
make compose-start   # start local stack
make controlp        # run control plane
make build-scaled    # build edge daemon binary
```

See the [Makefile](Makefile) for all targets and [docs/](docs/) for runtime documentation.
