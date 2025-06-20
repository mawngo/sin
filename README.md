# Sin

Backup tools

## Installation

Require go 1.24+.

```shell
go github.com/mawngo/sin@latest
```

Some command requires external tools present in your `$PATH`, which will be specified in the help message of that
command.

## Usage

This tool will first create a backup in the local directory then synchronize it to multiple remote targets.

The output backup filename specified by `--name` option.

Example command:

```shell
sin file example/mydirectory --config sync_file.json --name mybackup
```

Will create `mybackup.zip.bak` and sync it to targets specified in `sync_file.json`.

```shell
Backup created took 1.019ms
Start sync to 3 destinations
 SUCCESS  Synced to backup1 took 30.0182ms
 SUCCESS  Synced to backup2 took 15.8262ms
 SUCCESS  Skipped sync backup_long_term
Synced to 2 destinations
```

### Config File Format

To synchronize to multiple targets, you must specify a config
file using `--config` options.

```json5
{
    // Name of the backup process, this affects the output backup filename.
    // Optional, and can also be specified using `--name` option.
    // Can be overridden using `--name` option.
    "name": "backup_file",
    // Optional, Sentry DSN for error reporting.
    "sentryDSN": "https://<key>@sentry.io/<project-id>",
    // Optional, enable fail-fast mode, stop on sync error.
    // Can be overridden using `--ff` option.
    "failFast": false,
    // Optional, local backup directory, default to current directory.
    "backupTempDir": ".",
    // If true, the local backup will be kept, otherwise will be deleted after synced to targets.
    "keepTempFile": true,
    // Frequency of backup.
    // Accept crontab or duration. Run once if not specified.
    // End with `!` to run immediately on start.
    "frequency": "*/2 * * * *",
    // Default number of recent backups to keep.
    // Only apply for targets, local backup is always kept 0-1.
    // If not specified, or set to < 1, then keep unlimited.
    // Can be overridden using `--keep` option.
    "keep": 7,
    // Backup targets.
    "targets": [
        {
            // Name of the target, always required.
            "name": "backup1",
            // Optional, disable this target.
            "disabled": false,
            // Optional, override the default number of backup to keep above.
            "keep": 10,
            // Optional, only sync every N backups.
            // The first backup will always be synced.
            "each": 7,
            // Type of the target, always required.
            // Type affects other config options bellow. 
            // Supported: "file", "s3"
            "type": "file",
            // Required for "file" type.
            // Directory to sync backup to.
            "dir": "/media/backup/dir"
        },
        {
            "name": "s3backup_example",
            // ...
            // S3 specific config.
            "type": "s3",
            // Optional, base path prefix.
            "basePath": "test/dir",
            // Optional, S3 Region, default "auto".
            "region": "auto",
            // Optional, S3 Multipart config, only applied if the file >= thresholdMB.
            "multipart": {
                // Minimum size of the backup to switch to the multipart upload.
                // Min: 20MB, Max: 4GB.
                "thresholdMB": 110,
                // Optional, Upload part size in MB.
                // Min: 5MB, Max: 4GB.
                "partSizeMB": 50,
                // Optional, S3 Multipart concurrency.
                // If unset, let the library decide.
                "concurrency": 0,
                // Optional, disable checksum when multipart uploading.
                // This option must be true if you are using r2.
                "disableChecksum": false,
            },
            // S3 Bucket.
            "bucket": "???",
            // S3 Endpoint.
            "endpoint": "???",
            // S3 Access Key ID.
            "accessKeyID": "???",
            // S3 Access Secret.
            "accessSecret": "???"
        },
        {
            "name": "dryrun_example",
            // ...
            // Mock specific config, this target is for testing only.
            "type": "mock",
            // Optional, directory to write log to.
            "dir": ".",
            // Optional, log file name.
            // Instead of sync, "mock" will write a list of files to a file named by this field, or <name>.remote.log if not specified.
            "logFile": "mock.log"
        },
    ]
}
```

### Lockfile

Multiple instances of `sin` running with the same name to the same target will override each others,
which is usually unexpected.

To prevent this `sin` create a lock file to prevent multiple instances of same name to run at the same time, and will
remove the lock file on exit.

### Fail Fast Mode

By default, `sin` only exits when the backup generation process is failed, any errors happened during synchronization
will be reported, and it will continue running.

To exit on synchronization error, set `failFast` to true in the config file, or use `--ff` options.

### Pulling backups to local

Use `pull` command to download backup files to local machine.

```shell
sin pull --config sync_file.json --name mybackup
```

Config file must be specified and must contain at least one enabled target.
`pull` command using the same config file format as other backup commands:

```json5
{
    // Name of the backup file to download.
    "name": "backup_file",
    // Optional, Sentry DSN for error reporting.
    "sentryDSN": "https://<key>@sentry.io/<project-id>",
    // Optional, enable fail-fast mode, stop on sync error.
    // Can be overridden using `--ff` option.
    "failFast": false,
    // Optional, directory to download backup to, default to current directory.
    "backupTempDir": ".",
    // Frequency of pull.
    // Accept crontab or duration. Run once if not specified. 
    "frequency": "*/2 * * * *",
    // Default number of recent backups to keep on local.
    // If not specified, or set to < 1, then keep unlimited.
    // Can be overridden using `--keep` option.
    "keep": 7,
    // Backup sources.
    "targets": [
        {
            // Name of the target, always required.
            "name": "backup1",
            // Optional, disable this target.
            "disabled": false,
            // Target specific config....
        },
    ]
}
```

To see the list of available backups on remote target, use `list` command:

```shell
sin list --config sync_file.json --name mybackup
```

## Examples

Backup file/directory:

```shell
sin file example/file --config sync_file.json --name mybackup
```

Backup using mongodump:

```shell
sin mongo mongodb://localhost:27017 --config config.json --name testbackup_pg --gzip
```

Backup using pg_dump:

```shell
sin pg postgresql://localhost:5432 --config config.json --name testbackup_pg --gzip
```

Use `--help` for more details.

```
> sin --help
Backup tools

Usage:
  sin [command]

Available Commands:
  list        List remote backup files
  pull        Pull remote backup to local
  file        Run backup for file/directory
  mongo       Run backup for mongo using mongodump
  pg          Run backup for postgres using pg_dump
  help        Help about any command
  completion  Generate the autocompletion script for the specified shell

Flags:
  -c, --config string   specify config file
      --name string     name of output backup and log file
      --ff              enable fail-fast mode
      --keep int        number of backups to keep
      --env             (experimental) enable automatic environment binding
  -h, --help            help for sin

Use "sin [command] --help" for more information about a command.
```