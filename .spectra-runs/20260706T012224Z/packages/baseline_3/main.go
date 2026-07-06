package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"syscall"
)

var (
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonprinting bool
	showTabs        bool
)

type catState struct {
	newlines        int
	pendingCR       bool
	lineNumber      uint64
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonprinting bool
	showTabs        bool
}

func main() {
	files, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ostat, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: standard output: %v\n", err)
		os.Exit(1)
	}

	state := &catState{
		newlines:        0,
		pendingCR:       false,
		lineNumber:      0,
		number:          number,
		numberNonblank:  numberNonblank,
		squeezeBlank:    squeezeBlank,
		showEnds:        showEnds,
		showNonprinting: showNonprinting,
		showTabs:        showTabs,
	}

	ok := true
	for _, infile := range files {
		var file *os.File
		if infile == "-" {
			file = os.Stdin
		} else {
			var openErr error
			file, openErr = os.Open(infile)
			if openErr != nil {
				printError(infile, openErr)
				ok = false
				continue
			}
		}

		istat, statErr := file.Stat()
		if statErr != nil {
			printError(infile, statErr)
			if infile != "-" {
				file.Close()
			}
			ok = false
			continue
		}

		if istat.IsDir() {
			fmt.Fprintf(os.Stderr, "cat: %s: Is a directory\n", infile)
			if infile != "-" {
				file.Close()
			}
			ok = false
			continue
		}

		if sameInode(istat, ostat) {
			if istat.Mode().IsRegular() && ostat.Mode().IsRegular() {
				inPos, err1 := file.Seek(0, io.SeekCurrent)
				if err1 == nil {
					var outPos int64
					var err2 error
					isAppend := false
					if fd, err := syscall.Fcntl(int(os.Stdout.Fd()), syscall.F_GETFL, 0); err == nil {
						if fd&syscall.O_APPEND != 0 {
							isAppend = true
						}
					}
					if isAppend {
						currOutPos, errSeek := os.Stdout.Seek(0, io.SeekCurrent)
						if errSeek == nil {
							outPos, err2 = os.Stdout.Seek(0, io.SeekEnd)
							_, _ = os.Stdout.Seek(currOutPos, io.SeekStart)
						}
					} else {
						outPos, err2 = os.Stdout.Seek(0, io.SeekCurrent)
					}

					if err2 == nil && inPos < outPos {
						fmt.Fprintf(os.Stderr, "cat: %s: input file is output file\n", infile)
						if infile != "-" {
							file.Close()
						}
						ok = false
						continue
					}
				}
			}
		}

		if !(number || showEnds || showNonprinting || showTabs || squeezeBlank) {
			_, copyErr := io.Copy(os.Stdout, file)
			if copyErr != nil {
				printError(infile, copyErr)
				ok = false
			}
		} else {
			insize := ioBlksize(istat)
			outsize := ioBlksize(ostat)

			br := bufio.NewReaderSize(file, insize)
			bw := bufio.NewWriterSize(os.Stdout, outsize)

			catErr := catStream(br, bw, state)
			flushErr := bw.Flush()

			if catErr != nil && catErr != io.EOF {
				printError(infile, catErr)
				ok = false
			}
			if flushErr != nil {
				fmt.Fprintf(os.Stderr, "cat: write error: %v\n", flushErr)
				os.Exit(1)
			}
		}

		if infile != "-" {
			file.Close()
			}
	}

	if state.pendingCR {
		if _, err := os.Stdout.Write([]byte{'\r'}); err != nil {
			fmt.Fprintf(os.Stderr, "cat: write error: %v\n", err)
		os.Exit(1)
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func printError(infile string, err error) {
	if pe, ok := err.(*os.PathError); ok {
		fmt.Fprintf(os.Stderr, "cat: %s: %v\n", infile, pe.Err)
	} else {
		fmt.Fprintf(os.Stderr, "cat: %s: %v\n", infile, err)
	}
}

func sameInode(fi1, fi2 os.FileInfo) bool {
	s1, ok1 := fi1.Sys().(*syscall.Stat_t)
	s2, ok2 := fi2.Sys().(*syscall.Stat_t)
	if ok1 && ok2 {
		return s1.Dev == s2.Dev && s1.Ino == s2.Ino
	}
	return false
}

func ioBlksize(fi os.FileInfo) int {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		if stat.Blksize > 0 {
			return int(stat.Blksize)
		}
	}
	return 65536
}

func formatLineNumber(num uint64) []byte {
	var buf [24]byte
	idx := len(buf) - 1
	buf[idx] = '\t'
	idx--

	temp := num
	count := 0
	for temp > 0 {
		buf[idx] = byte('0' + (temp % 10))
		temp /= 10
		idx--
		count++
	}
	if count == 0 {
		buf[idx] = '0'
		idx--
		count++
	}

	for count < 6 {
		buf[idx] = ' '
		idx--
		count++
	}

	return buf[idx+1:]
}

func parseArgs() ([]string, error) {
	args := os.Args[1:]
	var files []string
	parsingOptions := true

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !parsingOptions {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			parsingOptions = false
			continue
		}
		if arg == "-" {
			files = append(files, arg)
			continue
		}
		if len(arg) > 1 && arg[0] == '-' {
			if arg[1] == '-' {
				switch arg {
				case "--number-nonblank":
					number = true
					numberNonblank = true
				case "--number":
					number = true
				case "--squeeze-blank":
					squeezeBlank = true
				case "--show-nonprinting":
					showNonprinting = true
				case "--show-ends":
					showEnds = true
				case "--show-tabs":
					showTabs = true
				case "--show-all":
					showNonprinting = true
					showEnds = true
					showTabs = true
				case "--help":
					printHelp()
					os.Exit(0)
				case "--version":
					printVersion()
					os.Exit(0)
				default:
					return nil, fmt.Errorf("cat: unrecognized option '%s'\nTry 'cat --help' for more information.", arg)
				}
			} else {
				for j := 1; j < len(arg); j++ {
					switch arg[j] {
					case 'b':
						number = true
						numberNonblank = true
					case 'e':
						showEnds = true
						showNonprinting = true
					case 'n':
						number = true
					case 's':
						squeezeBlank = true
					case 't':
						showTabs = true
						showNonprinting = true
					case 'u':
						// Ignored
					case 'v':
						showNonprinting = true
					case 'A':
						showNonprinting = true
						showEnds = true
						showTabs = true
					case 'E':
						showEnds = true
					case 'T':
						showTabs = true
					default:
						return nil, fmt.Errorf("cat: invalid option -- '%c'\nTry 'cat --help' for more information.", arg[j])
					}
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	if len(files) == 0 {
		files = append(files, "-")
	}

	return files, nil
}

func printHelp() {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Println("Concatenate FILE(s) to standard output.")
	fmt.Println()
	fmt.Println("With no FILE, or when FILE is -, read standard input.")
	fmt.Println()
	fmt.Println("  -A, --show-all           equivalent to -vET")
	fmt.Println("  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Println("  -e                       equivalent to -vE")
	fmt.Println("  -E, --show-ends          display $ or ^M$ at end of each line")
	fmt.Println("  -n, --number             number all output lines")
	fmt.Println("  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Println("  -t                       equivalent to -vT")
	fmt.Println("  -T, --show-tabs          display TAB characters as ^I")
	fmt.Println("  -u                       (ignored)")
	fmt.Println("  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Println("      --help     display this help and exit")
	fmt.Println("      --version  output version information and exit")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s f - g  Output f's contents, then standard input, then g's contents.\n", os.Args[0])
	fmt.Printf("  %s        Copy standard input to standard output.\n", os.Args[0])
}

func printVersion() {
	fmt.Println("cat (GNU coreutils) 9.5-reimplemented-in-go")
	fmt.Println("Copyright (C) 2026 Free Software Foundation, Inc.")
	fmt.Println("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.")
	fmt.Println("This is free software: you are free to change and redistribute.  There is NO WARRANTY, to the extent permitted by law.")
	fmt.Println()
	fmt.Println("Written by Torbjorn Granlund and Richard M. Stallman.")
}

func catStream(br *bufio.Reader, bw *bufio.Writer, state *catState) error {
	newlines := state.newlines
	pendingCR := state.pendingCR
	lineNumber := state.lineNumber

	writeStr := func(s string) {
		bw.WriteString(s)
	}
	writeByte := func(b byte) {
		bw.WriteByte(b)
	}

	ch, err := br.ReadByte()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	for {
		for ch == '\n' {
			newlines++
			if newlines > 0 {
				if newlines >= 2 {
					newlines = 2
					if state.squeezeBlank {
						ch, err = br.ReadByte()
						if err == io.EOF {
							state.newlines = newlines
							state.pendingCR = pendingCR
							state.lineNumber = lineNumber
							return nil
						}
						if err != nil {
							return err
						}
						continue
					}
				}
				if state.number && !state.numberNonblank {
					lineNumber++
					bw.Write(formatLineNumber(lineNumber))
				}
			}
			if state.showEnds {
				if pendingCR {
					writeStr("^M")
					pendingCR = false
				}
				writeByte('$')
			}
			writeByte('\n')

			ch, err = br.ReadByte()
			if err == io.EOF {
				state.newlines = newlines
				state.pendingCR = pendingCR
				state.lineNumber = lineNumber
				return nil
			}
			if err != nil {
				return err
			}
		}

		if pendingCR {
			writeByte('\r')
			pendingCR = false
		}

		if newlines >= 0 && state.number {
			lineNumber++
			bw.Write(formatLineNumber(lineNumber))
		}

		if state.showNonprinting {
			for {
				if ch >= 32 {
					if ch < 127 {
						writeByte(ch)
					} else if ch == 127 {
						writeStr("^?")
					} else {
						writeStr("M-")
						if ch >= 128+32 {
							if ch < 128+127 {
								writeByte(ch - 128)
							} else {
								writeStr("^?")
							}
						} else {
							writeStr("^")
							writeByte(ch - 128 + 64)
						}
					}
				} else if ch == '\t' && !state.showTabs {
					writeByte('\t')
				} else if ch == '\n' {
					newlines = -1
					break
				} else {
					writeStr("^")
					writeByte(ch + 64)
					}

				ch, err = br.ReadByte()
				if err == io.EOF {
					state.newlines = newlines
					state.pendingCR = pendingCR
					state.lineNumber = lineNumber
					return nil
				}
				if err != nil {
					return err
				}
			}
		} else {
			for {
				if ch == '\t' && state.showTabs {
					writeStr("^I")
				} else if ch != '\n' {
					if ch == '\r' && state.showEnds {
						nextBytes, peekErr := br.Peek(1)
						if peekErr == nil && nextBytes[0] == '\n' {
							pendingCR = true
						} else if peekErr == io.EOF {
							pendingCR = true
						} else {
							writeByte('\r')
						}
					} else {
						writeByte(ch)
					}
				} else {
					newlines = -1
					break
				}

				ch, err = br.ReadByte()
				if err == io.EOF {
					state.newlines = newlines
					state.pendingCR = pendingCR
					state.lineNumber = lineNumber
					return nil
				}
				if err != nil {
					return err
				}
			}
		}
	}
}
