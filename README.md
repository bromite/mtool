# mtool

mtool is a tool to snapshot, verify and restore file modification timestamp (mtime) of files in git repositories and it is designed to overcome a ninja build tool limitation;
see also: https://github.com/ninja-build/ninja/issues/1459

The typical use-case is restoring a timestamp known to the ninja build system after changing git branches so that - given the same original file content - the ninja build system
will not invalidte the intermediate and final build artifacts.

# How does it work?

The first step is to create an mtool snapshot; when creating the snapshot mtool expects the `git ls-files --stage` output
and will store relative filename, hash and file modification time in the mtool snapshot (either a file or standard output).

After you have done some git operations (changing branches, cherry-picking commits, rebasing etc) you may want to restore
the file modification timestamps you have previously saved if the file content did not change.

mtool will verify or change these modification timestamps if the file hash (as provided in the `git ls-files --stage` output)
did not change and if it differs from the one stored in the mtool snapshot.

# Usage

```
Usage: mtool [-achnrsv] [-i value] [-m value] [-o value] [parameters ...]
 -a, --append       Append to existing mtool snapshot, if any; only valid when
                    creating snapshot and when not using stdout
 -c, --create       create mtool snapshot; this is the default action
 -h, --help         display help and exit
 -i, --input=value  input filename; content is in 'git ls-files --stage' format;
                    - is for stdin (default)
 -m, --snapshot=value
                    mtool snapshot filename; - is for stdout (default)
 -n, --verify       verify that reference timestamps in current filesystem and
                    mtool snapshot are the same
 -o, --concurrency=value
                    how many goroutines to use for file mtime
                    verification/restore; 1024 is the default
 -r, --restore      restore reference timestamps into current filesystem from
                    mtool snapshot, if any changed
 -s, --ignore-missing
                    Ignore missing files during verify/restore
 -v, --verbose      Be verbose about mtime differences found during
                    verify/restore
```

See [chromium/mtime.sh](./chromium/mtime.sh) for a real-life use case.

All invocations expect the 'git ls-files --stage' output on stdin when `--input` is not specified.

# License

[GNU GPL v3](./LICENSE)
