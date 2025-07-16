# 2025-06-23 Auto-Tuning Distro Max Hosts

- status: proposed
- date: 2025-06-23
- authors: Kim Tao

## Context and Problem Statement

Each distro has a cap on the maximum number of hosts it can create to run tasks. There's two major reasons for this:

1. Some distros have to artificially keep the parallelization of tasks low (e.g. external non-Evergreen
   infrastructure that can only handle so many tasks running at once).
2. Limiting a distro's hosts is a safety guard to prevent any single distro from allocating too many hosts. This is
   important because Evergreen has (and will continue to have for the foreseeable future) a global limitation on the
   number of EC2 hosts it can have running at any given time.

However, even though this cap has its uses, it has also been somewhat inconvenient to maintain across many distros
because it has to be manually adjusted whenever a distro's usage changes. Distro max hosts isn't adjusted that
frequently, so it can cause long queue times unnecessarily if a distro has a max host limit that's far below what the
distro's tasks need to run quickly.

## Considered Options

One alternative that was considered was to make distro max hosts optional and allow those distros to scale up or down as
much as needed. Most distros are fairly low-usage with only a handful that are popular enough to potentially be a
concern for load, so it would reduce some burden from adjusting smaller distros. However, even then, having a cap on
distro max hosts could be useful just to prevent obvious over-usage. It would also not eliminate the problem entirely
because many of the more popular distros could benefit from being adjusted occasionally to allow more hosts and those do
have a concern that they could starve other distros of hosts during periods of very high load.

## Decision Outcome

The decision was to add a job that would incrementally increase/decrease a distro's max hosts according to that distro's
recent host usage. Keeping distro max hosts has the benefit of maintaining a cap on a distro to avoid starving other
distros. It also means a distro can more frequently adjust to actual distro usage since it runs as an automated
background process, which is an improvement over the purely human-driven process to manually adjust distro max hosts.
