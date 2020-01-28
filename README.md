# mysqltest

> Go library to spawn single-use MySQL / MariaDB servers for unit testing

[![Build Status](https://github.com/rubenv/mysqltest/workflows/Test/badge.svg)](https://github.com/rubenv/mysqltest/actions) [![GoDoc](https://godoc.org/github.com/rubenv/mysqltest?status.png)](https://godoc.org/github.com/rubenv/mysqltest)

Spawns a MySQL / MariaDB server with a single database configured. Ideal for unit
tests where you want a clean instance each time. Then clean up afterwards.

Features:

* Starts a clean isolated MySQL / MariaDB database
* Tested on Fedora and Ubuntu

## Usage

In your unit test:
```go
mysql, err := mysqltest.Start()
defer mysql.Stop()

// Do something with mysql.DB (which is a *sql.DB)
```

## License

This library is distributed under the [MIT](LICENSE) license.
