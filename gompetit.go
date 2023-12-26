package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func transformEncoding(rawReader io.Reader, trans transform.Transformer) string {
	ret, _ := io.ReadAll(transform.NewReader(rawReader, trans))
	return string(ret)
}

func fromShiftJIS(str string) string {
	return transformEncoding(strings.NewReader(str), japanese.ShiftJIS.NewDecoder())
}

func execCmd(q chan string, wg *sync.WaitGroup, cmd string, args []string, isCwd bool) {
	defer wg.Done()

	for path := range q {
		var arg []string
		if isCwd {
			arg = args
		} else {
			arg = append(args, path)
		}

		PrintLog(fmt.Sprintf("start: %s", path))
		defer PrintLog(fmt.Sprintf("done: %s", path))

		command := exec.Command(cmd, arg...)
		if isCwd {
			// refer "Goメモ-198 (*exec.Cmd 実行時にワーキングディレクトリを指定する) - いろいろ備忘録日記" https://devlights.hatenablog.com/entry/2022/04/22/073000
			command.Dir = path
		}
		out, err := command.CombinedOutput()

		prefix := filepath.Base(path)
		if err != nil {
			// err だと out が空になるわけではなかった。
			PrintLog(fmt.Sprintf("%s: %v", prefix, err))
		}
		if len(out) > 0 {
			PrintLog(fmt.Sprintf("%s: %s", prefix, fromShiftJIS(string(out))))
		}
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

func queue(q chan string, dirs []string, exts []string, isRecursive bool, isDir bool) {
	if !isRecursive {
		for _, path := range dirs {
			q <- path
		}
		return
	}

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

func getExts(str string) []string {
	if str == "" {
		return nil
	}

	exts := strings.Split(strings.ToLower(str), ",")
	for i, ext := range exts {
		exts[i] = "." + ext
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

	wg := new(sync.WaitGroup)
	q := make(chan string, 100)
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go execCmd(q, wg, cmd, args, isCwd)
	}

	queue(q, paths, exts, isRecursive, isDir)
	close(q)
	wg.Wait()
}

func getFileNameWithoutExt(path string) string {
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))])
}
func getFilename(ext string) string {
	exec, _ := os.Executable()
	return filepath.Join(filepath.Dir(exec), getFileNameWithoutExt(exec)+ext)
}
func PrintLog(m interface{}) {
	f, err := os.OpenFile(getFilename(".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic("Cannot open log file:" + err.Error())
	}
	defer f.Close()

	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime)
	log.Println(m)
}
