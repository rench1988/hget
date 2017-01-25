package main

import (
	"flag"
	"fmt"
	"httpdl"
	"os"
)

var (
	skipTls *bool

	rangeSize *uint

	connNum *uint
)

const (
	defSkipTls = true

	defRangeSize = 64 * 1024 * 1024 //64mb, must be same with nginx's slice config

	defConnNum = 128 //128 connection parallel download
)

func init() {
	skipTls = flag.Bool("skip-tls", defSkipTls, "skip verify certificate for https")

	rangeSize = flag.Uint("r", defRangeSize, "http request's range length(must be same with nginx's file slice config)")

	connNum = flag.Uint("c", defConnNum, "http connection num for parallel download")
}

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "tasks":
		if err := httpdl.DlTaskPrint(); err != nil {
			handlerError(err)
		}
	case "resume":
		if len(args) < 2 {
			usage()
			os.Exit(1)
		}
		if err := httpdl.DlTaskResume(args[1]); err != nil {
			handlerError(err)
		}
	default:
		if err := httpdl.DlTaskDo(nil, command, *rangeSize, int(*connNum), *skipTls); err != nil {
			handlerError(err)
		}
	}
}

func handlerError(err error) {
	fmt.Println(err)
	os.Exit(1)
}

func usage() {
	fmt.Println(`Usage:
            hget [URL] [-r range-size] [-skip-tls true]
            hget tasks
            hget resume [TaskName]
          `)
}
