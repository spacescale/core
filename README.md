[![CI](https://github.com/spacescale/core/actions/workflows/pipeline.yaml/badge.svg)](https://github.com/spacescale/core/actions/workflows/pipeline.yaml)
[![License](https://img.shields.io/github/license/spacescale/core)](./LICENSE)

# core

This repository contains the SpaceScale platform runtime — the control plane,
edge execution layer, and infrastructure tooling.

## Development

```bash
make compose-start   # start local stack
make controlp        # run control plane
make build-scaled    # build edge daemon binary
```

See the [Makefile](Makefile) for all targets and [docs/](docs/) for runtime documentation.
