## vmrestore

`vmrestore` restores data from backups created by [vmbackup](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/master/app/vmbackup/README.md).
VictoriaMetrics `v1.29.0` and newer versions must be used for working with the restored data.

Restore process can be interrupted at any time. It is automatically resumed from the inerruption point
when restarting `vmrestore` with the same args.


### Usage

VictoriaMetrics must be stopped during the restore process.

```
vmrestore -src=gcs://<bucket>/<path/to/backup> -storageDataPath=<local/path/to/restore>

```

* `<bucket>` is [GCS bucket](https://cloud.google.com/storage/docs/creating-buckets) name.
* `<path/to/backup>` is the path to backup made with [vmbackup](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/master/app/vmbackup/README.md) on GCS bucket.
* `<local/path/to/restore>` is the path to folder where data will be restored. This folder must be passed
  to VictoriaMetrics in `-storageDataPath` command-line flag after the restore process is complete.

The original `-storageDataPath` directory may contain old files. They will be susbstituted by the files from backup.


### Advanced usage

Run `vmrestore -help` in order to see all the available options:

```
vmrestore restores VictoriaMetrics data from backups made by vmbackup.

See the docs at https://github.com/VictoriaMetrics/VictoriaMetrics/blob/master/app/vmrestore/README.md .

  -concurrency int
    	The number of concurrent workers. Higher concurrency may reduce restore duration (default 10)
  -configFilePath string
    	Path to file with S3 configs. Configs are loaded from default location if not set.
    	See https://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html
  -credsFilePath string
    	Path to file with GCS or S3 credentials. Credentials are loaded from default locations if not set.
    	See https://cloud.google.com/iam/docs/creating-managing-service-account-keys and https://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html
  -loggerLevel string
    	Minimum level of errors to log. Possible values: INFO, ERROR, FATAL, PANIC (default "INFO")
  -memory.allowedPercent float
    	Allowed percent of system memory VictoriaMetrics caches may occupy (default 60)
  -src string
    	Source path with backup on the remote storage. Example: gcs://bucket/path/to/backup/dir, s3://bucket/path/to/backup/dir or fs:///path/to/local/backup/dir
  -storageDataPath string
    	Destination path where backup must be restored. VictoriaMetrics must be stopped when restoring from backup. -storageDataPath dir can be non-empty. In this case only missing data is downloaded from backup (default "victoria-metrics-data")
  -version
    	Show VictoriaMetrics version
```


### How to build from sources

It is recommended using [binary releases](https://github.com/VictoriaMetrics/VictoriaMetrics/releases) - see `vmutils-*` archives there.


#### Development build

1. [Install Go](https://golang.org/doc/install). The minimum supported version is Go 1.12.
2. Run `make vmrestore` from the root folder of the repository.
   It builds `vmrestore` binary and puts it into the `bin` folder.

#### Production build

1. [Install docker](https://docs.docker.com/install/).
2. Run `make vmrestore-prod` from the root folder of the repository.
   It builds `vmrestore-prod` binary and puts it into the `bin` folder.

#### Building docker images

Run `make package-vmrestore`. It builds `victoriametrics/vmrestore:<PKG_TAG>` docker image locally.
`<PKG_TAG>` is auto-generated image tag, which depends on source code in the repository.
The `<PKG_TAG>` may be manually set via `PKG_TAG=foobar make package-vmrestore`.
