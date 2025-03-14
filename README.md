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
sin file /mydirectory --config sync_file.json --name mybackup
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
    "name": "backup_file",
    // Optional, local backup directory, default to current directory.
    "backupTempDir": ".",
    // If true, the local backup will be kept, otherwise will be deleted after synced to targets.
    "keepTempFile": true,
    // Frequency of backup.
    // Accept crontab or duration. Run once if not specified. 
    "frequency": "*/2 * * * *",
    // Default number of recent backups to keep.
    // Only apply for targets, local backup is always kept 0-1.
    // If not specified, or set to < 1, then keep unlimited.
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
            // Optional, Upload part size in MB.
            "partSizeMB": 100,
            // S3 Bucket.
            "bucket": "???",
            // S3 Endpoint.
            "endpoint": "???",
            // S3 Access Key ID.
            "accessKeyID": "???",
            // S3 Access Secret.
            "accessSecret": "???"
        },
    ]
}
```
