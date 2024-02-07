# Best Practices

## Secrets

Note that you should not pass secrets as command-line arguments but instead as
environment variables or from a file, as Evergreen runs `ps` periodically, which
will log command-line arguments. You can use the `silent` parameter in
`shell.exec` or `subprocess.exec` to avoid logging output.

## Task Directory

Evergreen creates a temporary task directory for each task. Commands by default execute in that directory. Evergreen will clean up that directory in between tasks unless explicit configured not to. Please don't write outside this directory, as Evergreen won't be able to delete the data your task has written.

## subprocess.exec

In general, use [subprocess.exec](Project-Commands.md#subprocessexec) instead of shell.exec.

The reasons to prefer subprocess.exec include:
1. Evergreen uses expansions with the same syntax as shell expansions.
2. The shell isn't always bash. Debian/Ubuntu sh is dash.
3. Debugging inline shell scripts is hard.
4. When they're inline, it becomes hard for people to execute them themselves, which makes it difficult to reproduce what Evergreen has done locally.

You can pass environment variables to subprocess.exec if you'd like to pass expansions. It's a good idea to avoid shell.exec as much as possible.


## shell.exec

While subprocess.exec is a safer option that we recommend you use instead, it's important to understand Evergeen's behavior and expectations if you do need to use shell.exec.

### Unsafe expansion use

Please ensure you have tested your tasks (including by checking the Agent Logs tab) to ensure that you are not accidentally leaking a sensitive expansion via shell.exec

```yml
  functions:
    "publish":
    	- command: shell.exec
    	   params:
        	script: |
        	  set +x
            AWS_SECRET=${aws_token}
            set -x
```

The above Evergreen configuration snippet is unsafe because all lines in a shell.exec snippet will be logged to Evergreen’s system logs with their expansions populated. Note that even using shell settings like +x/-x does not prevent Evergreen from logging these lines. There are a few ways to make the above block safe, listed below.

#### Set the “silent: true” shell.exec parameter

```yml
  functions:
    "publish":
    	- command: shell.exec
    	   params:
          silent: true
        	script: |
        	  set +x
            AWS_SECRET=${aws_token}
            set -x
```

This prevents Evergreen from automatically capturing all shell.exec lines

#### Move the shell.exec script to a dedicated file

```sh
$ cat ./dedicated-file.sh
set +x
AWS_SECRET=${aws_token}
set -x
```

```yml
  functions:
    "publish":
    	 - command: shell.exec
    	   params:
         # any of the following three parameters will fix this problem
         env:
           "aws_token": ${aws_token}
         include_expansions_in_env:
          - "aws_token"
         add_expansions_to_env: true
  	     script: |
           ./dedicated-file.sh
```

Evergreen will only log the line that invokes the script, not the lines of the dedicated script.
You could replace the inline script with only a call to “./dedicated-file.sh”. Expansions are not accessible to scripts invoked this way by default, but setting the “env”, “add_expansions_to_env”, or “include_expansions_in_env” parameters appropriately can fix this.

## Task Tags

Use [task tags](Project-Configuration-Files.md#task-and-variant-tags) to reduce repetition in your Evergreen configuration file.

## Expansions

Be cautious about Evergreen's expansion syntax.

Evergreen chose an expansion syntax that unfortunately conflicts with bash. This means that you cannot do something like this.

```bash
# WRONG
export foo=bar
echo ${foo}
```

Evergreen will first look for an expansion called `foo` and will substitute that expansion, or, if it doesn't exist, the empty string. You must drop the curly braces if you would like to use a bash variable.

```bash
export foo=bar
echo $foo
```

## Distro Choice

Tasks on more popular distros are often run quicker than tasks on less popular ones. Prefer more popular distros where possible. For more information about available distro choices see [Guidelines around Evergreen distros](https://wiki.corp.mongodb.com/x/CZ7yBg)
