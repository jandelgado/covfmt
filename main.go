package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/build"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type block struct {
	startLine  int
	startChar  int
	endLine    int
	endChar    int
	statements int
	covered    int
}

var vscDirs = []string{".git", ".hg", ".bzr", ".svn"}

type cacheResult struct {
	file string
	err  error
}

var pkgCache = map[string]cacheResult{}

func findFile(file string) (string, error) {
	dir, file := filepath.Split(file)
	if cached, ok := pkgCache[dir]; ok {
		return cached.file, cached.err
	}

	var result cacheResult
	pkg, err := build.Import(dir, ".", build.FindOnly)
	if err != nil {
		err = fmt.Errorf("can't find %q: %v", file, err)
		result = cacheResult{"", err}
	} else {
		result = cacheResult{filepath.Join(pkg.Dir, file), nil}
	}
	pkgCache[dir] = result
	return result.file, result.err
}

func findRepositoryRoot(dir string) (string, bool) {
	for _, vcsdir := range vscDirs {
		if d, err := os.Stat(filepath.Join(dir, vcsdir)); err == nil && d.IsDir() {
			return dir, true
		}
	}
	nextdir := filepath.Dir(dir)
	if nextdir == dir {
		return "", false
	}
	return findRepositoryRoot(nextdir)
}

func getCoverallsSourceFileName(name string) string {
	if dir, ok := findRepositoryRoot(name); !ok {
		return name
	} else {
		filename := strings.TrimPrefix(name, dir+string(os.PathSeparator))
		return filename
	}
}

func writeLcovRecord(filePath string, blocks []*block, w *bufio.Writer) {

	w.WriteString("TN:\n")
	w.WriteString("SF:" + filePath + "\n")

	// Loop over functions
	// FN: line,name

	// FNF: total functions
	// FNH: covered functions

	// Loop over functions
	// FNDA: stats,name ?

	// Loop over lines
	total := 0
	covered := 0

	// Loop over each block and extract the lcov data needed.
	for _, b := range blocks {
		// For each line in a block we add an lcov entry and count the lines.
		for i := b.startLine; i <= b.endLine; i++ {
			total++
			if b.covered > 0 {
				covered++
			}
			w.WriteString("DA:" + strconv.Itoa(i) + "," + strconv.Itoa(b.covered) + "\n")
		}
	}

	w.WriteString("LF:" + strconv.Itoa(total) + "\n")
	w.WriteString("LH:" + strconv.Itoa(covered) + "\n")

	// Loop over branches
	// BRDA: ?

	// BRF: total branches
	// BRH: covered branches

	w.WriteString("end_of_record\n")
}

func lcov(blocks map[string][]*block, f io.Writer) {
	w := bufio.NewWriter(f)
	for file, fileBlocks := range blocks {
		writeLcovRecord(file, fileBlocks, w)
	}
	w.Flush()
}

// Format being parsed is:
//   name.go:line.column,line.column numberOfStatements count
// e.g.
//   github.com/jandelgado/golang-ci-template/main.go:6.14,8.2 1 1

func parseCoverageLine(line string) (string, *block, bool) {
	if strings.HasPrefix(line, "mode:") {
		return "", nil, false
	}
	path := strings.Split(line, ":")
	if len(path) != 2 {
		return "", nil, false
	}
	parts := strings.Split(path[1], " ")
	if len(parts) != 3 {
		return "", nil, false
	}
	sections := strings.Split(parts[0], ",")
	if len(sections) != 2 {
		return "", nil, false
	}
	start := strings.Split(sections[0], ".")
	end := strings.Split(sections[1], ".")
	// Populate the block.
	b := &block{}
	b.startLine, _ = strconv.Atoi(start[0])
	b.startChar, _ = strconv.Atoi(start[1])
	b.endLine, _ = strconv.Atoi(end[0])
	b.endChar, _ = strconv.Atoi(end[1])
	b.statements, _ = strconv.Atoi(parts[1])
	b.covered, _ = strconv.Atoi(parts[2])
	f, err := findFile(path[0])
	if err != nil {
		return "", nil, false
	}
	return getCoverallsSourceFileName(f), b, true
}

func parseCoverage(coverage io.Reader) map[string][]*block {
	scanner := bufio.NewScanner(coverage)
	blocks := map[string][]*block{}
	for scanner.Scan() {
		if f, b, ok := parseCoverageLine(scanner.Text()); ok {
			// Make sure the filePath is a key in the map.
			if _, ok := blocks[f]; ok == false {
				blocks[f] = []*block{}
			}
			blocks[f] = append(blocks[f], b)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(scanner.Err())
	}
	return blocks
}

func main() {
	infileName := flag.String("coverin", "", "If supplied, use a go cover profile (comma separated)")
	outfileName := flag.String("lcovout", "", "If supplied, use a go cover profile (comma separated)")

	flag.Parse()
	if len(flag.Args()) > 0 {
		cmd := os.Args[0]
		s := "Usage: %s [options]\n"
		fmt.Fprintf(os.Stderr, s, cmd)
		flag.PrintDefaults()
		//	flag.Usage()
		os.Exit(1)
	}

	infile := os.Stdin
	outfile := os.Stdout
	var err error
	if *infileName != "" {
		infile, err = os.Open(*infileName)
		if err != nil {
			panic(err)
		}
		defer infile.Close()
	}
	if *outfileName != "" {
		outfile, err = os.Create(*outfileName)
		if err != nil {
			panic(err)
		}
		defer outfile.Close()
	}

	lcov(parseCoverage(infile), outfile)
}
