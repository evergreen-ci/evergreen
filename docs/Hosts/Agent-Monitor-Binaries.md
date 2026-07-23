# Evergreen Binaries on Tasks

Evergreen tasks may encounter multiple Evergreen binaries on their host. The agent monitor is a long-lived process that downloads and supervises the rolling agent binary used to execute tasks.

These paths are internal to Evergreen.

| Path                        | Role                                                                         | Supported for callers?              |
| --------------------------- | ---------------------------------------------------------------------------- | ----------------------------------- |
| `~/evergreen_agent_monitor` | Long-lived agent monitor binary (provision, host setup, Jasper monitor loop) | No — internal Evergreen use only    |
| `<client_dir>/evergreen`    | Rolling agent binary downloaded by the monitor on each version change        | No — internal Evergreen use only    |
| `~/evergreen`               | Compatibility copy of the current agent for workflows that still invoke it   | **No** — refreshed best-effort only |

Some existing tasks invoke `~/evergreen` directly. The agent monitor refreshes this compatibility path best-effort whenever it downloads a new agent, but new callers should not rely on it being available or current.

**Do not build new features or task commands against `~/evergreen_agent_monitor`.** Migrate callers to supported interfaces: use the [static client download manifest](../CLI.md#static-client-download-manifest) to discover the latest platform-specific S3 URL, then download the Evergreen binary inside your task's directory and use that download.
