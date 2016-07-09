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

func transformEncoding(rawReader io.Reader, trans transform.Transformer) string {
	ret, _ := ioutil.ReadAll(transform.NewReader(rawReader, trans))
	return string(ret)
}

func fromShiftJIS(str string) string {
	return transformEncoding(strings.NewReader(str), japanese.ShiftJIS.NewDecoder())
}

func startWalker(q chan string, stdout chan string, wg *sync.WaitGroup, cmd string, args []string) {
	defer wg.Done()

	for {
		path, ok := <-q
		if !ok {
			return
		}

		arg := append(args, path)
		stdout <- fmt.Sprintf("start: %s %s", cmd, strings.Join(arg, " "))

		out, err := exec.Command(cmd, arg...).Output()
		var str = fmt.Sprintf("done: %s", path) + "\n" + fromShiftJIS(string(out))
		if err != nil {
			str = str + "\n" + fmt.Sprintf("%v", err)
		}
		stdout <- str
	}
}

func matchExts(path string, exts []string) bool {
	for _, ext := range exts {
		if strings.ToLower(filepath.Ext(path)) == ext {
			return true
		}
	}
	return false
}

func execWalkFunc(q chan string, exts []string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if exts == nil || matchExts(path, exts) {
			q <- path
		}
		return nil
	}
}

func queueRecursiveFile(q chan string, dirs []string, exts []string) {
	for _, dir := range dirs {
		err := filepath.Walk(dir, execWalkFunc(q, exts))
		if err != nil {
			fmt.Println(err)
		}
	}
}

func queuePath(q chan string, paths []string) {
	for _, path := range paths {
		q <- path
	}
}

func getExts(str string) []string {
	var exts []string

	if str == "" {
		exts = nil
	} else {
		exts = strings.Split(strings.ToLower(str), ",")
		for i, ext := range exts {
			exts[i] = "." + ext
		}
	}
	return exts
}

func getPaths(strs []string) []string {
	var paths []string

	for _, path := range strs {
		if _, err := os.Stat(path); err != nil {
			fmt.Println("Stat err ", path, ": ", err)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] cmd \"cmdargs\" path...\nOPTIONS:\n", os.Args[0])
		flag.PrintDefaults()
	}
	var (
		parallel    int
		isRecursive bool
		extsStr     string
	)
	flag.IntVar(&parallel, "P", 2, "同時実行数")
	flag.BoolVar(&isRecursive, "r", false, "path のディレクトリを辿ってファイルを対象とする。")
	flag.StringVar(&extsStr, "e", "", "-r を指定した場合に処理対象のファイル拡張子を指定。, で複数指定（スペースは挟まない）。例: -e png,jpg")

	flag.Parse()

	if flag.NArg() < 3 {
		flag.Usage()
		os.Exit(1)
	}
	cmd := flag.Arg(0)
	args := strings.Fields(flag.Arg(1))
	exts := getExts(extsStr)
	paths := getPaths(flag.Args()[2:])

	stdout := make(chan string)
	go func() {
		for {
			str, ok := <-stdout
			if !ok {
				return
			}

			fmt.Println(str)
		}
	}()

	var wg sync.WaitGroup
	q := make(chan string)
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go startWalker(q, stdout, &wg, cmd, args)
	}

	if isRecursive {
		queueRecursiveFile(q, paths, exts)
	} else {
		queuePath(q, paths)
	}
	close(q)
	wg.Wait()
	close(stdout)
}
