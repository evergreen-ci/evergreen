How to set up and use the new patch tool
==

###Client

Basic Usage
--

* To submit a patch, run this from your local copy of the mongodb/mongo repo:

      `evergreen patch -p <project-id>`
    
NOTE: The first time you run this, you'll be asked if you want to set these as your default project and variants.
After setting defaults, you can omit the flags and the default values will be used, so that just running `evergreen patch` will work.

Defaults may be changed at any time by editing your `~/.evergreen.yml` file.

Extra args to the `git diff` command used to generate your patch may be specified by appending them after `--`.  For example, to generate a patch relative to the previous commit:

      evergreen patch -- HEAD~1

Or to patch relative to a specific tag:

      evergreen patch -- r3.0.2

For patches containing binary data, you can use the extra args to git diff to include the binary changes by passing through --binary to the `git` command itself:

      evergreen patch -- --binary

Operating on existing patches
--


* To list patches you've created:

      `evergreen list-patches`


* To cancel a patch:
 
	  `evergreen cancel-patch -i <patch_id>`
    
* To finalize a patch:
 
      `evergreen finalize-patch -i <patch_id>`


* To add changes to a module on top of an existing  patch:

     ```
      cd ~/projects/module-project-directory
      evergreen set-module -i <patch_id> -m <module-name>
      ```

### Server Side (for evergreen admins)

To enable auto-updating of client binaries, add a section like this to the settings file for your server:


```yaml
api:
    clients:
        latest_revision: "c0110ba937047f99c9a68470f6ec51fc6d98c5cc"
        client_binaries:
           - os: "darwin"
             arch: "amd64"
             url: "https://.../evergreen"
           - os: "linux"
             arch: "amd64"
             url: "https://.../evergreen"
           - os: "windows"
             arch: "amd64"
             url: "https://.../evergreen"
```

The "url" keys in each list item should contain the appropriate URL to the binary for each architecture. The "latest_revision" key should contain the githash that was used to build the binary. It should match the output of "evergreen version" for *all* the binaries at the URLs listed in order for auto-updates to be successful.
