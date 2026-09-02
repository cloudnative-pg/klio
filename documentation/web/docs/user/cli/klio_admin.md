---
title: klio admin
---

## klio admin

Server administration commands

### Options

```
  -h, --help   help for admin
```

### Options inherited from parent commands

```
      --config string                     config file (default is $HOME/.klio.yaml)
      --debug                             enable debug logging
      --log-destination string            where the log stream will be written
      --log-field-level string            JSON log field to report severity in (default: level)
      --log-field-timestamp string        JSON log field to report timestamp in (default: ts)
      --log-level string                  the desired log level, one of error, info, debug and trace (default "info")
      --log-truncate-destination          truncate the log destination on open instead of appending to it (ignored for FIFOs)
      --pprof-server string               enable the PPROF server using the specified address
      --zap-devel                         Development Mode defaults(encoder=consoleEncoder,logLevel=Debug,stackTraceLevel=Warn). Production Mode defaults(encoder=jsonEncoder,logLevel=Info,stackTraceLevel=Error)
      --zap-encoder encoder               Zap log encoding (one of 'json' or 'console')
      --zap-log-level level               Zap Level to configure the verbosity of logging. Can be one of 'debug', 'info', 'error', 'panic' or any integer value > 0 which corresponds to custom debug levels of increasing verbosity
      --zap-stacktrace-level level        Zap Level at and above which stacktraces are captured (one of 'info', 'error', 'panic').
      --zap-time-encoding time-encoding   Zap time encoding (one of 'epoch', 'millis', 'nano', 'iso8601', 'rfc3339' or 'rfc3339nano'). Defaults to 'epoch'.
```

### SEE ALSO

* [klio](klio.md)	 - PostgreSQL Backup & Recovery for CloudNativePG
* [klio admin delete-backup](klio_admin_delete-backup.md)	 - Delete a backup from the Klio server
* [klio admin list-backups](klio_admin_list-backups.md)	 - List the backups available in the Klio server
* [klio admin queue](klio_admin_queue.md)	 - Manage the queue tasks
* [klio admin refresh](klio_admin_refresh.md)	 - Refresh the Kopia cache and policies

