# Spawn Hosts

If a test fails on a platform other than the one you develop on locally, you'll likely want to get access to a machine of that type in order to investigate the source of the failure. You can accomplish this using the spawn hosts feature of evergreen.

## Troubleshooting connecting to a spawn host

If you are having trouble connecting to a spawn host:

- Make sure you are connected to the MongoDB network or VPN.
- Verify the host is in the "RUNNING" state and the DNS host name for the spawn host is correct. Note that if you pause and then start the host, it will get a new DNS name.
- Verify you are using the correct user. This can be found on the [My Hosts](https://spruce.mongodb.com/spawn/host) page under the Spawn Host detail drop down.
- Verify you are using the correct ssh key used when creating the Spawn Host. Use the -i argument to specify the location of your local private ssh key to use.
- Newer versions of macOS do not by default support older SSH algorithms. Please add these lines to the `Host *` stanza of your ~/.ssh/config:

```
  Host *
      HostkeyAlgorithms +ssh-rsa
      PubkeyAcceptedAlgorithms +ssh-rsa
```

## Making a distro "spawnable"

Evergreen administrators can choose to make a distro available to users for spawning by checking the box on the distro configuration panel labeled _"Allow users to spawn these hosts for personal use"_

Only distros backed by a provider that supports dynamically spinning up new hosts (static hosts, of course, do not) allow this option.

## Spawning a Host

Visit `/spawn` to view the spawn hosts control panel. Click on "Spawn Host" and choose the distro you want to spawn, and choose the key you'd like to use (or provide a new one).

## Spawning a Host From a Task

Alternately, for a task that ran on a distro where spawning is enabled, you will see a "Spawn..." or "Spawn Host" link on its task page.

![task_page_spawn_host.png](../images/task_page_spawn_host.png)

Clicking it will pre-populate the spawn host page with a request to spawn a host of that distro, along with the option to fetch binaries and artifacts associated with the task and any tasks that it depended on.

![spawn_host_modal.png](../images/spawn_host_modal.png)

Fetching artifacts can also be performed manually; see [fetch](../CLI.md#fetch) in the Evergreen command line tool documentation.

Artifacts are placed in /data/mci. Note that you will likely be able to ssh into the host before the artifacts are finished fetching.

If your project has a project setup script defined at the admin level, you can also check "Use project-specific setup script defined at ..." before creating the spawn host. You can check if there are errors fetching artifacts or running this script on the host page: `https://spruce.mongodb.com/host/<host_id>`.

EC2 spawn hosts can be stopped/started and modified from the Spawn Host page, or via the command line, which is documented in [Basic Host Usage](../CLI.md#basic-host-usage) in the Evergreen command line tool documentation.

## Spawn Host Expiration

By default, spawn hosts expire after one week. This expiration can be set (or the host can be made unexpirable) when
spawning the host or can be set later by pressing the "edit" button for the host. You can extend an expirable host's
lifetime up to 30 days past host creation.

If you'd like to get a notification before a host expires, you can [set up a
notification](../Project-Configuration/Notifications.md#spawn-host-expiration) for it.

## Hosts Page

The spruce hosts page shows all hosts and offers three batch actions applicable to hosts:

1. Update Status
   You can force a state change to these statuses:
   -Decomissioned: Terminate a host running tasks after it's done running its current task.
   -Quarantined: Stop a host from running tasks without terminating it or shutting it down.
   -Terminate: Actually terminate the host.
   -Stopped: Stop it.
   -Runnning: Start it back up.

2. Restart Jasper
   Try forcing the Evergreen agent (which runs in a system process called Jasper) to start back up in a way that's less disruptive than just rebooting the computer.

3. Reprovision
   Hosts need to have a few starter files and stuff on the file system before they can run tasks, and pressing that button gives them the files they need if they're a static host(because there's no way to auto-populate them with the stuff they need). This option is useful for static hosts that get extremely borked (like the file system is corrupted and it loses all the data).
