# Evergreen Limits

Evergreen has different kinds of limits in place. The ones that users are likely to encounter are listed here.

## Non-expirable Hosts

Evergreen limits users to two unexpirable spawn hosts at a time.

#### Can an exception be requested?

Exceptions can be requested on a case by case which will be granted based on [our policy](https://mongodb.stackenterprise.co/questions/1122).

## Task Limits

Evergreen limits tasks per version to 40,000.

## Task Timeout Max

Evergreen does not have a max on how large task timeouts can be.

## Include Limits

There is an [investigation](https://jira.mongodb.org/browse/DEVPROD-3509) to figure out how many include files we want to let users put in their project configuration. The current limit is 50.

## YAML configuration size

ELarge parser projects are disabled, which means that Evergreen limits YAML project configuration sizes to 16MB. This includes configurations that get added with generate tasks.

#### Can an exception be requested?

This cannot be lifted because Evergreen cannot safely handle larger file sizes from multiple users. Users are encouraged to instead try creating a new patch with fewer variants/tasks.

## Patch size

Evergreen has no limits on patch size (the number of files changed or the diff size for the changes). In order to submit patches that are larger than 16MB from the cli, the --large
flag needs to be used. While we don’t block large patches, they may hit the CLI’s one minute timeout. To work around that, users can open a PR and run the patch from there.

## What is the largest file size I can upload to Parsley?

Evergreen does not have a limit for Parsley file sizes. However, you will encounter a limit based on the browser that you are using. As of April 4, 2022 the limits are:

In V8 (used by Chrome and Node), the maximum length is 229 - 24 (~1GiB). On 32-bit systems, the maximum length is 228 - 16 (~512MiB). In Firefox, the maximum length is 230 - 2 (~2GiB). Before Firefox 65, the maximum length was 228 - 1 (~512MiB). In Safari, the maximum length is 231 - 1 (~4GiB).
