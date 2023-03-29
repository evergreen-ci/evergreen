# Evergreen Best Practices

## Task Directory

Evergreen creates a temporary task directory for each task. Commands by default execute in that directory. Evergreen will clean up that directory in between tasks unless explicit configured not to. Please don't write outside this directory, as Evergreen won't be able to delete the data your task has written.

## subprocess.exec

In general, use [subprocess.exec](https://github.com/evergreen-ci/evergreen/wiki/Project-Commands#subprocess-exec) instead of shell.exec.

The reasons to prefer subprocess.exec include:
1. Evergreen uses expansions with the same syntax as shell expansions.
2. The shell isn't always bash. Debian/Ubuntu sh is dash.
3. Debugging inline shell scripts is hard.
4. When they're inline, it becomes hard for people to execute them themselves, which makes it difficult to reproduce what Evergreen has done locally.

You can pass environment variables to subprocess.exec if you'd like to pass expansions. It's a good idea to avoid shell.exec as much as possible.

## Task Tags

Use [task tags](https://github.com/evergreen-ci/evergreen/wiki/Project-Configuration-Files#task-tags) to reduce repetition in your Evergreen configuration file.

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
