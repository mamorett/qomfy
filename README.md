# qomfy

A CLI tool written in Go to submit [ComfyUI](https://www.comfy.org/) workflows
and download their outputs. It is the 1:1 Go rewrite of the Python script
`cc.py`, with the same commands, flags, defaults, and behavior.

Workflows ship with the project under `workflows/` and are installed into
`~/.config/qomfy/workflows/` (overridable via `--workflows-dir` or the config
`workflows_dir` entry).

## Build & install

```bash
# build for host (also cross-compiles all configured os/arch)
./godelw build qomfy

# install the bundled workflows into ~/.config/qomfy/workflows
./build/install-workflows.sh

# (optional) install into a custom dir
./build/install-workflows.sh /path/to/workflows

# run
./out/build/qomfy/qomfy --help
```

## Config

```bash
# create ~/.config/qomfy/config.json with a server URL
qomfy config init --server-url http://localhost:8188 --force

# print the resolved config path / workflows directory
qomfy config path
qomfy config workflows
```

Config resolution order (highest → lowest priority):

1. `--config <path>`
2. `QOMFY_CONFIG` env var
3. `$XDG_CONFIG_HOME/qomfy/config.json`
4. `~/.config/qomfy/config.json`

## Commands

`health`, `send`, `wait`, `run`, `text-to-image`, `image-text-to-image`,
`image-to-glb`, `rig-glb`, `text-to-glb`, `text-to-rigged-glb`, and the
`config` subcommands (`init`, `path`, `workflows`).

All commands accept the global flags `--config`, `--client-id`,
`--poll-interval`, `--timeout`, `--verbose`, `--workflows-dir`, and
`--downloads-dir`.

See `qomfy <command> --help` for per-command flags.
