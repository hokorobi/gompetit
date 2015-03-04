package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

var (
	parallel    int
	args        []string
	cmd         string
	extsStr     string
	exts        []string
	isRecursive bool
)

func transformEncoding(rawReader io.Reader, trans transform.Transformer) string {
	ret, _ := ioutil.ReadAll(transform.NewReader(rawReader, trans))
	return string(ret)
}

func FromShiftJIS(str string) string {
	return transformEncoding(strings.NewReader(str), japanese.ShiftJIS.NewDecoder())
}

func execFunc(path string, limit chan bool, stdout chan string, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
		<-limit
	}()
	stdout <- fmt.Sprintf("start: %s %s %s", cmd, strings.Join(args, " "), path)

	out, err := exec.Command(cmd, append(args, path)...).Output()
	var str = fmt.Sprintf("done: %s", path) + "\n" + FromShiftJIS(string(out))
	if err != nil {
		str = str + "\n" + fmt.Sprintf("%v", err)
	}
	stdout <- str
}

func matchExts(path string, exts []string) bool {
	for _, ext := range exts {
		if strings.ToLower(filepath.Ext(path)) == ext {
			return true
		}
	}
	return false
}

func execWalkFunc(limit chan bool, stdout chan string, wg *sync.WaitGroup) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if exts == nil || matchExts(path, exts) {
			limit <- true
			wg.Add(1)
			go execFunc(path, limit, stdout, wg)
			return nil
		}
		return nil
	}
}

func execRecursivFile(dirs []string, stdout chan string, wg *sync.WaitGroup) {
	limit := make(chan bool, parallel)
	for _, dir := range dirs {
		err := filepath.Walk(dir, execWalkFunc(limit, stdout, wg))
		if err != nil {
			fmt.Println(err)
		}
	}
}

func execPath(paths []string, stdout chan string, wg *sync.WaitGroup) {
	limit := make(chan bool, parallel)
	for _, path := range paths {
		limit <- true
		wg.Add(1)
		go execFunc(path, limit, stdout, wg)
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] cmd \"cmdargs\" path...\nOPTIONS:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.IntVar(&parallel, "P", 2, "同時実行数")
	flag.BoolVar(&isRecursive, "r", false, "path のディレクトリを辿ってファイルを対象とする。")
	flag.StringVar(&extsStr, "e", "", "-r を指定した場合に処理対象のファイル拡張子を指定。, で複数指定（スペースは挟まない）。例: -e png,jpg")

	flag.Parse()

	if flag.NArg() < 3 {
		flag.Usage()
		os.Exit(1)
	}
	cmd = flag.Arg(0)
	args = strings.Fields(flag.Arg(1))
	if extsStr == "" {
		exts = nil
	} else {
		exts = strings.Split(strings.ToLower(extsStr), ",")
		for i, ext := range exts {
			exts[i] = "." + ext
		}
	}

	var paths []string
	for _, path := range flag.Args()[2:] {
		if _, err := os.Stat(path); err != nil {
			fmt.Println("Stat err ", path, ": ", err)
			continue
		}
		paths = append(paths, path)
	}

	workerquit := make(chan bool)
	taskquit := make(chan bool)
	stdout := make(chan string)

	go func() {
	loop:
		for {
			select {
			case <-taskquit:
				workerquit <- true
				break loop
			case s := <-stdout:
				fmt.Println(s)
			}
		}
	}()

	var wg sync.WaitGroup
	if isRecursive {
		execRecursivFile(paths, stdout, &wg)
	} else {
		execPath(paths, stdout, &wg)
	}
	wg.Wait()
	taskquit <- true

	<-workerquit
}
