# Boxer — Project Instructions

## Sandbox networking

Never copy host files into container filesystems or resolv configurations.
Host files like `/etc/resolv.conf` reflect the host's local setup (e.g. systemd-resolved stub at `127.0.0.53`, a local DNS/proxy server) and are meaningless or harmful inside an isolated network namespace.

For container DNS: use explicit public resolvers (8.8.8.8 / 8.8.4.4) or allow the operator to override via config/env. Do not read from the host.
