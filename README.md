# slogger

Slogger is a logging library for Go.

Slogger supports multiple appenders including an asynchronous appender
and a rolling file appender.

## Installing

Slogger can be installed via `go get` :
```
go get github.com/mongodb/slogger/v2/slogger
```

## Example

At the core of slogger is the Logger type.  Here is an example where
we create a Logger with a StringAppender.

```go
package main

import (
	"github.com/mongodb/slogger/v2/slogger"

	"bytes"
	"fmt"
)

func main() {
	buffer := new(bytes.Buffer)
	appender := slogger.StringAppender{buffer}
	logger := slogger.Logger{"", []slogger.Appender{appender}, 0, nil}

	logger.Logf(slogger.OFF, "Here is a sample log line: %v", 1)
	logger.Logf(slogger.OFF, "Here's another one: %v", 2)
	fmt.Print(buffer)
}
```

The above code prints:

```
[2016/02/25 14:35:10.168] [.off] [proc.c:main:247] Here is a sample log line: 1
[2016/02/25 14:35:10.168] [.off] [proc.c:main:247] Here's another one: 2
```

Some of slogger's appenders can be composed.  For example,
`StringAppender` does not respect logging levels, but we can create a
`FilterAppender` that does.  We can compose them to create an appender
that pays attention to logging levels and outputs to a string.

```go
package main

import (
	"github.com/mongodb/slogger/v2/slogger"

	"bytes"
	"fmt"
)

func main() {
	buffer := new(bytes.Buffer)
	appender := slogger.LevelFilter(slogger.INFO, slogger.StringAppender{buffer})
	logger := slogger.Logger{"", []slogger.Appender{appender}, 0, nil}

	logger.Logf(slogger.INFO, "This log line will make it through")
	logger.Logf(slogger.DEBUG, "This log line won't")
	fmt.Print(buffer)
}
```

The above code prints:

```
[2016/02/25 14:41:56.420] [.info] [slogger2.go:main:15] This log line will make it through
```

Other appenders include an AsyncAppender, a
RetainingLevelFilterAppender, and a RollingFileAppender.  See the code
for details.

## Contributing

1. Sign the [MongoDB Contributor Agreement](https://www.mongodb.com/legal/contributor-agreement).
2. Fork the repository.
3. Make your changes.
4. Ensure that the tests pass by running `./run-tests`.
5. Submit a pull request.

## License

Slogger is made available under the Apache License version 2.
