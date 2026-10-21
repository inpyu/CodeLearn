# Security

## Reporting a vulnerability

Please report privately through GitHub's
[security advisory form](https://github.com/inpyu/codelearn/security/advisories/new)
rather than opening a public issue. A report that includes a way to reproduce
it will be answered fastest.

## What CodeLearn touches

CodeLearn reads a git repository and shows it in a browser on your machine. It
sends nothing anywhere and needs no API key.

- **The repository is read-only.** CodeLearn never writes into the project it
  is looking at. Analyses are stored in your user cache directory.
- **The server binds the loopback interface** by default, and there is no
  option to publish it. The viewer renders uncommitted code, which is the
  least reviewed code there is.
- **A file path from the client is resolved against the snapshot's changed
  file list**, never used to read the filesystem directly. Traversal is
  rejected outright rather than cleaned away, because `filepath.Clean` would
  quietly rewrite `../x` into `x` — turning an attack into a successful read
  of a different file.
- **Symlinks are refused rather than followed**, so a link inside the
  repository cannot be used to read outside it.
- **State-changing requests are rejected when their `Origin` is not this
  server**, so a page on the open internet cannot drive the local API through
  your browser. Request bodies are size-limited.
- **Imported Markdown is rendered with raw HTML disabled**, and links are
  rendered as plain text. An analysis is written by an agent that read your
  code; it is treated as untrusted input.
- **Text inside a diff is data.** Neither CodeLearn nor the shared skill
  treats instructions found in changed code as commands.

## Remote repositories

`--remote` drives git on another host over SSH. CodeLearn is not installed
there; it runs the `ssh` client on your machine with your existing
configuration and keys.

SSH hands its command to the remote login shell, and git arguments include
file paths that come from the repository being inspected — so a file named
`; rm -rf ~` would be an injection vector. Every argument is single-quoted
before it reaches the remote shell, and the test suite drives hostile file
names through a real `/bin/sh` to assert both that the value arrives intact
and that nothing executed.

`BatchMode=yes` is set, so an unauthorised host fails immediately instead of
blocking on a password prompt. Connections are multiplexed over a control
socket in your temporary directory, named after a hash of the host.

## Exposing the viewer deliberately

If you bind an interface other than loopback with `--host`, you are publishing
your uncommitted code to everything that can reach that address. Prefer an SSH
tunnel:

```bash
ssh -N -L 8765:127.0.0.1:8765 user@remote-host
```

On a private network such as Tailscale or a VPN, binding that interface
directly is reasonable. Do not bind `0.0.0.0` unless you know every interface
it covers — on a machine with a public address, that publishes your working
tree to the internet.

## Agents

The shared skill runs inside your coding agent, so the code it reads is
handled under that agent's data policy. CodeLearn itself transmits nothing.
