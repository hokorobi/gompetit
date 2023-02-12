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
	"time"

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

func startWalker(q chan string, stdout chan string, wg *sync.WaitGroup, cmd string, args []string, isCwd bool) {
	defer wg.Done()

	for path := range q {
		var arg []string
		if isCwd {
			err := os.Chdir(path)
			if err != nil {
				stdout <- fmt.Sprintf("%v", err)
				continue
			}
			arg = args
		} else {
			arg = append(args, path)
		}

		stdout <- fmt.Sprintf("start %s: %s", time.Now().Format("15:04:05"), path)
		prefix := filepath.Base(path)

		execCmd := exec.Command(cmd, arg...)
		out, err := execCmd.CombinedOutput()
		if err != nil {
			// err だと out が空になるわけではなかった。
			stdout <- fmt.Sprintf("%s: %s", prefix, fromShiftJIS(string(out)))
			stdout <- fmt.Sprintf("%s: %v", prefix, err)
		} else {
			stdout <- fmt.Sprintf("%s: %s", prefix, fromShiftJIS(string(out)))
		}
		stdout <- fmt.Sprintf("done %s: %s", time.Now().Format("15:04:05"), path)
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

func execWalkFuncDir(q chan string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			q <- path
		}
		return nil
	}
}

func queueRecursive(q chan string, dirs []string, exts []string, isDir bool) {
	var err error
	for _, dir := range dirs {
		if isDir {
			err = filepath.Walk(dir, execWalkFuncDir(q))
		} else {
			err = filepath.Walk(dir, execWalkFunc(q, exts))
		}
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

	for _, str := range strs {
		path, err := filepath.Abs(str)
		if err != nil {
			fmt.Println("Abs err ", str, ": ", err)
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
		isDir       bool
		isCwd       bool
	)
	flag.IntVar(&parallel, "P", 2, "同時実行数")
	flag.BoolVar(&isRecursive, "r", false, "path のディレクトリを辿ってファイルを対象とする。")
	flag.StringVar(&extsStr, "e", "", "-r を指定した場合に処理対象のファイル拡張子を指定。, で複数指定（スペースは挟まない）。 -d との併用不可。例: -e png,jpg")
	flag.BoolVar(&isDir, "d", false, "ディレクトリを処理対象とする。 -e との併用不可。")
	flag.BoolVar(&isCwd, "c", false, "-r と -d を指定した場合に見つけたディレクトリをカレントディレクトリとして処理を実行する。")
	// TODO: オプションの整合性確認
	// -r, -d なし -c の処理がうまくいかないのでは？

	flag.Parse()

	if flag.NArg() < 3 {
		flag.Usage()
		os.Exit(1)
	}
	if isDir && extsStr != "" {
		flag.Usage()
		os.Exit(1)
	}
	cmd := flag.Arg(0)
	args := strings.Fields(flag.Arg(1))
	exts := getExts(extsStr)
	paths := getPaths(flag.Args()[2:])

	stdout := make(chan string, 100)
	go func() {
		for str := range stdout {
			fmt.Println(str)
		}
	}()

	wg := new(sync.WaitGroup)
	q := make(chan string, 100)
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go startWalker(q, stdout, wg, cmd, args, isCwd)
	}

	if isRecursive {
		queueRecursive(q, paths, exts, isDir)
	} else {
		queuePath(q, paths)
	}
	close(q)
	wg.Wait()

	// FIXME: 最後の stdout への出力が捨てられてしまう。
	// stdout から取得する前に close している？
	close(stdout)
}
