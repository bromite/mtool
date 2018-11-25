package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"./getopt"
)

var (
	nonVerifyingFiles, totalFiles uint64
	verbose                       bool
)

func main() {
	var (
		help, verify, restore, create bool
		append                        bool
		inputFileName                 = "-"
		mtoolFileName                 = "-"
		concurrency                   = 1024
	)

	getopt.StringVarLong(&inputFileName, "input", 'i', "input filename; content is in 'git ls-files --stage' format; - is for stdin (default)")
	getopt.StringVarLong(&mtoolFileName, "snapshot", 'm', "mtool snapshot filename; - is for stdout (default)")
	getopt.BoolVarLong(&append, "append", 'a', "Append to existing mtool snapshot, if any; only valid when creating snapshot and when not using stdout")
	getopt.BoolVarLong(&verbose, "verbose", 'v', "Be verbose about mtime differences found during verify/restore")
	getopt.BoolVarLong(&help, "help", 'h', "display help and exit")
	getopt.BoolVarLong(&create, "create", 'c', "create mtool snapshot; this is the default action")
	getopt.BoolVarLong(&verify, "verify", 'n', "verify that reference timestamps in current filesystem and mtool snapshot are the same")
	getopt.BoolVarLong(&restore, "restore", 'r', "restore reference timestamps into current filesystem from mtool snapshot, if any changed")
	getopt.IntVarLong(&concurrency, "concurrency", 'o', "how many goroutines to use for file mtime verification/restore; 1024 is the default")

	getopt.Parse()

	if len(getopt.Args()) != 0 {
		getopt.Usage()
		os.Exit(1)
	}

	if concurrency < 1 {
		fmt.Fprintf(os.Stderr, "ERROR: concurrency should be 1 or more\n")
		os.Exit(1)
	}

	cmd := 0
	if verify {
		cmd++
	}
	if restore {
		cmd++
	}
	if create {
		cmd++
	}
	if help {
		cmd++
	}

	switch cmd {
	case 0:
		create = true
	case 1:
		// ok
		if help {
			getopt.Usage()
			return
		}
	default:
		fmt.Fprintf(os.Stderr, "ERROR: specify one command\n")
		getopt.Usage()
		os.Exit(10)
	}

	var src io.Reader
	if inputFileName != "-" {
		f, err := os.Open(inputFileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(15)
		}
		src = f
		defer f.Close()
	} else {
		src = os.Stdin
	}

	inputScanner := bufio.NewScanner(src)

	switch {
	case create:
		var dst io.Writer
		if mtoolFileName != "-" {
			if append {
				f, err := os.OpenFile(mtoolFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					os.Exit(15)
				}
				defer f.Close()
				dst = f
			} else {
				f, err := os.Create(mtoolFileName)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					os.Exit(15)
				}
				defer f.Close()
				dst = f
			}
		} else {
			dst = os.Stdout
		}
		mustScanGitLsInput(inputScanner, func(_ int, fileName, sha1 string, mtime time.Time) error {
			_, err := fmt.Fprintf(dst, "%s\t%s\t%d\n", fileName, sha1, mtime.UnixNano())
			return err
		})
		return
	case restore, verify:
		if inputFileName == mtoolFileName {
			fmt.Fprintf(os.Stderr, "ERROR: input and snapshot cannot be the same when restoring\n")
			os.Exit(10)
		}
		var mtoolSrc io.Reader
		if mtoolFileName != "-" {
			f, err := os.Open(mtoolFileName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				os.Exit(15)
			}
			mtoolSrc = f
			defer f.Close()
		} else {
			mtoolSrc = os.Stdin
		}
		// load the mtool snapshot
		m := mustLoadMtool(bufio.NewScanner(mtoolSrc))

		// create a pool of goroutines
		res := make(chan struct{}, concurrency)
		for i := 0; i < concurrency; i++ {
			res <- struct{}{}
		}

		var wg sync.WaitGroup
		mustScanGitLsInput(inputScanner, func(lineNo int, fileName, sha1 string, mtime time.Time) error {
			wg.Add(1)
			go func() {
				defer func() {
					wg.Done()
					res <- struct{}{}
				}()
				<-res
				err := restoreCallback(m, fileName, sha1, mtime, verify)
				if err != nil {
					// stop at first error
					fmt.Fprintf(os.Stderr, "ERROR: line %d: %v\n", lineNo, err)
					os.Exit(20)
				}
			}()

			return nil
		})

		// wait for all goroutines to complete
		wg.Wait()

		if verbose {
			fmt.Fprintf(os.Stderr, "mtool: %d/%d files verified successfully\n", totalFiles-nonVerifyingFiles, totalFiles)
		}

		if verify {
			// return 0 only when no changes are needed
			os.Exit(int(nonVerifyingFiles))
		}

		return
	}
	panic("NOT REACHED")
}

func restoreCallback(m map[string]*entry, fileName, sha1 string, mtime time.Time, verifyOnly bool) error {
	// first verify if file is in the mtool snapshot and hash did not change
	var expectedMtime time.Time
	if e, ok := m[fileName]; !ok {
		return nil
	} else {
		// not deleting from map since it is accessed concurrently
		if e.sha1 != sha1 {
			return nil
		}
		expectedMtime = e.mtime
	}

	atomic.AddUint64(&totalFiles, 1)

	foundMtime := mtime.UnixNano()
	if foundMtime != expectedMtime.UnixNano() {
		atomic.AddUint64(&nonVerifyingFiles, 1)
		if verifyOnly {
			if verbose {
				fmt.Fprintf(os.Stderr, "%s: modified time expected %d but found %d\n", fileName, expectedMtime.UnixNano(), foundMtime)
			}
		} else {
			// set the mtime; atime is not preserved
			err := os.Chtimes(fileName, time.Now(), expectedMtime)
			if err == nil && verbose {
				fmt.Fprintf(os.Stderr, "%s: changed modified time from %v to %v\n", fileName, mtime, expectedMtime)
			}
			return err
		}
	}
	return nil
}

type entry struct {
	sha1  string
	mtime time.Time
}

func mustLoadMtool(scanner *bufio.Scanner) map[string]*entry {
	m := map[string]*entry{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		parts := strings.SplitN(strings.TrimSpace(scanner.Text()), "\t", 3)

		if len(parts) != 3 {
			fmt.Fprintf(os.Stderr, "ERROR: line %d: expected 3 tab-separated fields for restore/verify, got %d\n", lineNo, len(parts))
			os.Exit(10)
		}

		mtime, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(15)
		}

		fileName, sha1 := parts[0], parts[1]

		if _, ok := m[fileName]; ok {
			fmt.Fprintf(os.Stderr, "ERROR: line %d: filename collision in mtool snapshot for %q\n", lineNo, fileName)
			os.Exit(10)
		}

		m[fileName] = &entry{
			sha1:  sha1,
			mtime: time.Unix(0, mtime),
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(15)
	}

	return m
}

type gitLsCallback func(lineNo int, fileName, sha1 string, mtime time.Time) error

func mustScanGitLsInput(scanner *bufio.Scanner, fn gitLsCallback) {
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		parts := strings.SplitN(strings.TrimSpace(scanner.Text()), "\t", 2)

		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "ERROR: line %d: expected 2 tab-separated fields in 'git ls-files --stage' output\n", lineNo)
			os.Exit(10)
		}

		fileName := parts[1]
		s, err := os.Stat(fileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(15)
		}
		mtime := s.ModTime()

		parts = strings.SplitN(parts[0], " ", 3)
		if len(parts) != 3 {
			fmt.Fprintf(os.Stderr, "ERROR: line %d: malformed first field\n", lineNo)
			os.Exit(10)
		}

		err = fn(lineNo, fileName, parts[1], mtime)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: line %d: %v\n", lineNo, err)
			os.Exit(20)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(15)
	}
}
