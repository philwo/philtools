// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// A simple, very fast version of the netcat utility.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

const (
	RandomBufSize = 1 * 1024 * 1024 // 1MB
)

var (
	useIPv4    = flag.Bool("4", false, "use IPv4 only")
	useIPv6    = flag.Bool("6", false, "use IPv6 only")
	listen     = flag.Bool("l", false, "listen mode")
	source     = flag.String("s", "", "source address to bind to")
	verbose    = flag.Bool("v", false, "verbose mode")
	inputFile  = flag.String("i", "", "input file")
	outputFile = flag.String("o", "", "output file")

	network     string // tcp, tcp4 or tcp6
	destination string // host:port
)

func main() {
	progName := filepath.Base(os.Args[0])
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [OPTIONS] [destination] [port]\n", progName)
		flag.PrintDefaults()
		os.Exit(1)
	}

	if err := parseFlags(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", progName, err)
		flag.Usage()
	}

	// Determine the network type.
	switch {
	case *useIPv4:
		network = "tcp4"
	case *useIPv6:
		network = "tcp6"
	default:
		network = "tcp"
	}

	// Start the actual work.
	var err error
	if *listen {
		err = listenMode()
	} else {
		err = connectMode()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", progName, err)
		os.Exit(1)
	}
}

func parseFlags() error {
	flag.Parse()

	if flag.NArg() != 2 {
		return errors.New("invalid number of arguments")
	}

	host := flag.Arg(0)
	if host == "" {
		return errors.New("destination must be specified")
	}

	port := flag.Arg(1)
	if port == "" {
		return errors.New("port must be specified")
	}

	destination = net.JoinHostPort(host, port)

	if *useIPv4 && *useIPv6 {
		return errors.New("cannot specify both -4 and -6")
	}

	if *listen && *source != "" {
		return errors.New("cannot specify source address in listen mode")
	}

	if *listen && *inputFile != "" {
		return errors.New("cannot specify input file in listen mode")
	}

	if !*listen && *outputFile != "" {
		return errors.New("cannot specify output file in connect mode")
	}

	return nil
}

func listenMode() (err error) {
	// Listen on the specified address and port.
	listener, err := net.Listen(network, destination)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer listener.Close()

	if *verbose {
		fmt.Fprintf(os.Stderr, "listening on %v\n", listener.Addr())
	}

	// Accept a connection.
	conn, err := listener.Accept()
	if err != nil {
		return fmt.Errorf("failed to accept connection: %w", err)
	}
	defer conn.Close()

	if *verbose {
		fmt.Fprintf(os.Stderr, "connection from %v\n", conn.RemoteAddr())
	}

	var writer io.WriteCloser = os.Stdout
	if *outputFile != "" {
		writer, err = os.Create(*outputFile)
		if err != nil {
			return fmt.Errorf("failed to open file for writing: %w", err)
		}

		defer func() {
			if tmpErr := writer.Close(); tmpErr != nil && err == nil {
				err = fmt.Errorf("failed to close file: %w", tmpErr)
			}
		}()
	}

	if _, err = io.Copy(writer, conn); err != nil {
		return fmt.Errorf("failed to read from connection: %w", err)
	}

	return nil
}

func connectMode() (err error) {
	dialer := &net.Dialer{}

	// Ensure we bind to the specified source address, if specified.
	if *source != "" {
		dialer.LocalAddr, err = net.ResolveTCPAddr(network, net.JoinHostPort(*source, "0"))
		if err != nil {
			return fmt.Errorf("failed to resolve source address: %w", err)
		}

		if *verbose {
			fmt.Fprintf(os.Stderr, "binding to %v as source address\n", dialer.LocalAddr)
		}
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "connecting to %s\n", destination)
	}

	// Dial the specified address and port.
	conn, err := dialer.Dial(network, destination)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	if *verbose {
		fmt.Fprintf(os.Stderr, "connected to %v\n", conn.RemoteAddr().String())
	}

	var reader io.ReadCloser = os.Stdin
	if *inputFile != "" {
		reader, err = os.Open(*inputFile)
		if err != nil {
			return fmt.Errorf("failed to open file for reading: %w", err)
		}

		defer func() {
			if tmpErr := reader.Close(); tmpErr != nil && err == nil {
				err = fmt.Errorf("failed to close file: %w", tmpErr)
			}
		}()
	}

	// Read all data from stdin and write it to the connection.
	if rt, ok := conn.(io.ReaderFrom); ok {
		_, err = rt.ReadFrom(reader)
	} else {
		_, err = io.Copy(conn, reader)
	}

	if err != nil {
		return fmt.Errorf("failed to write to connection: %w", err)
	}

	return nil
}
