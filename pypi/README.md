# meshclaw-go

`pip install meshclaw-go` installs a thin launcher that downloads the official
`meshclaw` Go binary for your platform from
[GitHub Releases](https://github.com/zeus-kim/meshclaw-public/releases) on first
run and forwards all arguments to it.

```sh
pip install meshclaw-go
meshclaw --version
```

The binary is cached in `~/.local/bin/meshclaw`. To build from source instead,
see the repository root README (`go build -o ./bin/meshclaw ./cmd/meshclaw`).
