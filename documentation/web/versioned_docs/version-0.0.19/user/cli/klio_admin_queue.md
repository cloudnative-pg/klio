---
title: klio admin queue
---

## klio admin queue

Manage the queue tasks

### Options

```
  -h, --help                 help for queue
      --json                 Output in JSON format
      --socket-path string   Unix socket used by the administration server (default "/tmp/.klio-admin")
```

### Options inherited from parent commands

```
      --config string                     config file (default is $HOME/.klio.yaml)
      --debug                             enable debug logging
      --log-destination string            where the log stream will be written
      --log-field-level string            JSON log field to report severity in (default: level)
      --log-field-timestamp string        JSON log field to report timestamp in (default: ts)
      --log-level string                  the desired log level, one of error, info, debug and trace (default "info")
      --pprof-server string               enable the PPROF server using the specified address
      --zap-devel                         Development Mode defaults(encoder=consoleEncoder,logLevel=Debug,stackTraceLevel=Warn). Production Mode defaults(encoder=jsonEncoder,logLevel=Info,stackTraceLevel=Error)
      --zap-encoder encoder               Zap log encoding (one of 'json' or 'console')
      --zap-log-level level               Zap Level to configure the verbosity of logging. Can be one of 'debug', 'info', 'error', 'panic' or any integer value > 0 which corresponds to custom debug levels of increasing verbosity
      --zap-stacktrace-level level        Zap Level at and above which stacktraces are captured (one of 'info', 'error', 'panic').
      --zap-time-encoding time-encoding   Zap time encoding (one of 'epoch', 'millis', 'nano', 'iso8601', 'rfc3339' or 'rfc3339nano'). Defaults to 'epoch'.
```

### SEE ALSO

* [klio admin](klio_admin.md)	 - Server administration commands
* [klio admin queue backup](klio_admin_queue_backup.md)	 - Manage the queue backup tasks
* [klio admin queue status](klio_admin_queue_status.md)	 - Show the status of the task queue (pending backups and pending WALs)
* [klio admin queue wal](klio_admin_queue_wal.md)	 - Manage the queue WAL tasks

