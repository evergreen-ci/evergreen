# Containerized Tasks Overview

We're excited to introduce the ability to run Evergreen tasks in
containers!

This offering is designed to streamline work and reduce friction caused
by software dependency requirements. Greater flexibility and control
over task environments is achievable with containers, ensuring that each
task runs in an isolated, dedicated space with its own specific set of
software dependencies.

## Important Note

Container tasks at this time are still an experimental feature,
therefore they are subject to change as we iterate further on our
roadmap. The feature may have bugs that get discovered as we roll it out
as an initial offering.

If you have any questions about container tasks or are interested in
exploring how this feature could benefit your project, we encourage you
to reach out to us in ***#evergreen-users***. We'll discuss its
potential applications and assist you in preparing for its broader
release.

It's important to distinguish that this feature is entirely separate
from the existing functionality Evergreen has to spin up docker hosts
via the host.create command, as this feature is designed to run entire
tasks within containers, rather than spinning them up within the
existing model.

This feature is also distinct from the existing distro-based Docker
pools. These were created with the aim of running containers within our
existing host pool by having containers operate the Evergreen agent
inside our EC2 hosts that we initiated. However, this solution was slow
due to the necessity of the container distro rebuilding its image every
time the agent was released, which led to increased latency. It also
lacked flexibility of image choice, and each host could only run a
maximum of one container at a time, and the same container was also used
across tasks, leading to reusability issues.

This offering aims to be an improvement that addresses these issues by
leveraging the advantages of native containerization, enabling better
speed, flexibility, and efficiency for running tasks.

## What's Different

While tasks running on containers come with the same general
capabilities you're familiar with in regular host tasks, ultimately they
are not hosts themselves, meaning certain low-level system processes
such as systemd present on the host that may not be readily available
within a container. To learn more, the [docker security
documentation](https://docs.docker.com/engine/security/) explains the
features of Docker's isolation model. There are a couple of differences
to note as we roll out this new feature:

1.  Task Groups: The initial release of container tasks does not support
configuring task groups. This is something we aim to support in
future iterations.

2.  Priority: The priority setting feature will not be available during
the initial release for container tasks.

3.  Greater configurability: When running container tasks, resources
such as CPU and memory usage must be explicitly configured, unlike
in the current distro model. Users are also responsible for
picking the image to use, and all required software is downloaded
during runtime. Furthermore, the container your task runs on is
dedicated solely to that task and is never reused, so you are free
to do whatever you want with it without needing to worry about
leaving the environment in a messy state for the next task.

# How to get started
## Image lifecycle and self service

At the moment, our image security automation is still [under
development](https://docs.google.com/document/d/1MMePuL5YBjJQcNdtwzU2kMLPSsRLzDyE0rhTVkmXDqo/edit#heading=h.ghiwibz2opmd),
so for the time being, modifying a project\'s respective Dockerfile
requires the submission of a PR against our repo.

1.  Stage a PR in our centralized image
    [repository](https://github.com/evergreen-ci/container-initial-offering-dockerfiles).
    This repo hosts Dockerfiles for all teams participating in the
    beta of running container tasks in Evergreen. The PR, when merged
    into the main branch, triggers an automated Drone pipeline which
    then builds the image and pushes it to Amazon Elastic Container
    Registry (ECR). To onboard, your PR changes must include a
    relevant Dockerfile, created under a new directory that has the
    name of your project. Review should be requested from
    **evergreen-ci/evg-app**, and the PR's approval will be contingent
    on the Dockerfile following the [secure image
    policy](https://docs.google.com/document/d/1MMePuL5YBjJQcNdtwzU2kMLPSsRLzDyE0rhTVkmXDqo/edit)
    that we currently have in place.

2.  Once the PR has been approved and is merged into main with the Drone
    build passing, you may reference the URI of the newly built image.
    Its format will be as follows:
    
    > **557821124784.dkr.ecr.us-east-1.amazonaws.com/evergreen/${directory}:${sha}**

    Where **\${dir}** is your project's directory name in our image
    repository. For enhanced security, we need to use immutable image
    tags, so rather than the typical \":latest\" tag, image URIs in your
    Evergreen YAML must also use a **\${sha}** tag, corresponding to the
    desired commit in our image repository.

## YAML Configuration

Configuring your project to use container tasks is done in YAML.
Container definitions are similar to distro configurations in that they
both are ultimately referenced in the **run_on** field of a build
variant. However, container configurations are defined by the user,
rather than distros which are configured by Evergreen admins.

Below is an example setup for configuring a build variant to run
container tasks:

``` yaml
containers:
  - name: example-container
    working_dir: /
    image: "557821124784.dkr.ecr.us-east-1.amazonaws.com/evergreen/your_repo:<hash>"
    resources:
      cpu: 1024
      memory_mb: 2048
    system:
      cpu_architecture: x86_64
      operating_system: linux
      
  - name: example-small-container
    working_dir: /
    image: "557821124784.dkr.ecr.us-east-1.amazonaws.com/evergreen/other_repo:<hash>"
    size: small-container
    system:
      cpu_architecture: x86_64
      operating_system: linux
```

Fields:

-   **name**: a user-defined name for the container that represents the
    task or the environment of the container

-   **working_dir**: the working directory for your tasks within the
    container. In the example, it's set to the root directory

-   **image**: the Docker image to use for the container. Initially,
    this must be one of our pre-approved base images. Users will be
    able to submit Dockerfiles to us for review, at which point we'll
    build them into a container registry. Defining arbitrary
    Dockerfiles will be unsupported to start as we need to vet them as
    we scope out the best image-building primitives that are both
    sustainable and secure.

-   **resources**: the resources allocated to the container: cpu and
    memory_mb set the CPU units and the memory (in MB), respectively,
    that the container is allocated

-   **size**: an alternative to the resources section, a preset size for
    the container configured within the UI

-   **system**: specification for the CPU architecture and the operating
    system to be used by your container (currently linux is the only
    supported operating system)

Once containers are configured, they must be referenced by a build
variant. Example:

``` yaml
buildvariants:
  - name: container-variant
    display_name: Container Variant
    run_on:
      - example-container
    tasks:
      - name: test-graphql
      - name: test-js
```

The container name must be put in the **run_on** field, in the same way
that a distro may alternatively be put there. Unlike the distro model,
where a primary and secondary distro can be specified in this field
(hence why the field is a list), only one container may be specified in
the **run_on** field for a containerized variant. If more than one
container name is specified, only the first entry will be recognized.

### UI Changes

Once configured properly, a variant with container tasks is ready to
schedule tasks. Once tasks get created, key differences to look for in
Spruce are:

#### **Container Project Settings**

A new tab has been added to the project settings page for container
configurations. Users can create a list of resource configuration
presets that can be referenced via alias in the size field of their
container YAML configurations.

![containers.png](../images/container_configuration.png)

Options:

-   Name: The alias for the resource preset. Names must be unique within
    the list.

-   Memory: The amount of memory (in MiB) to allocate.

-   CPU: The CPU units the container can use. These values are expressed
    in 'vCPU Units'. 1024 CPU units is the equivalent of 1vCPU.

Users can define as many container configurations as needed, reflecting
different appropriate resource needs for various tasks.

#### **Task Metadata**

A link to a container task's respective container replaces the typical
host link.

<img alt="containerized_task_metadata.png" height="400" src="../images/container_metadata.png" width="300"/>

#### **Container Page**

The link in the task metadata sidebar takes you to the container page,
which details the lifecycle of a container and their tasks. Like the
host page, event logs exist charting the journey of a container task
from initialization to termination. Task events such as container
assignment and status changes are also recorded, as well as the clearing
of a task from a container once it has run its course.

![container_event_logs.png](../images/container_page.png)

### Other Considerations

### Disk Space

Each instance is provisioned with 200GB of space; however, the actual
disk space available for each container task can vary depending on the
number and the nature of tasks sharing the same instance. In this sense,
while CPU and memory are isolated allocations to each container, disk
space is a shared resource across all containers on the same host. As
such, dedicating more memory and CPU for a container makes it less
likely to share the instance's disk space with other containers.
Conversely, less resource-hungry containers are more likely to share the
instance with others, so they will likely have a smaller share of the
available disk space.

In practical terms, this means that while each instance has a maximum of
200GB of disk space, please bear in mind that the effective disk space
available to your container tasks might be less and fluctuate, as other
containers take and release disk space as they get created and exit,
respectively. While we work on a more robust solution to this notion of
isolating disk space, we recommend that you keep your container task
disk space usage to a maximum of 10GB.

### Git Cloning

In a containerized environment, the hosts themselves are unable to be
distributed with SSH keys used to clone GitHub repositories. This means
that if git clones rely on the host's SSH keys to clone a private
repository in your existing workflow, the cloning method will need to be
changed to cloning via OAuth over HTTPS.

The
[git.get_project](https://docs.devprod.prod.corp.mongodb.com/evergreen/Project-Configuration/Project-Commands#gitget_project)
command *can* be configured to pass an OAuth token to be used for
cloning the repository, but if no token field is specified Evergreen has
its own general purpose OAuth token which will be used to attempt to
clone the repository. Because of this, for most cases a token does not
need to be set in YAML, unless the repository cannot be cloned with
Evergreen's token.

Module repos that are configured to clone via SSH will need to have
their url format modified to HTTPS, e.g:

```yaml
modules:
  - name: evergreen
    repo: git@github.com:evergreen-ci/evergreen.git → https://github.com/evergreen-ci/evergreen.git
```
