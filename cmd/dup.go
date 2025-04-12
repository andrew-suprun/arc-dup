package main

import (
	"fmt"
	"log"
	"os"

	"dup/app"
	"dup/fs"
	"dup/fs/mockfs"
	"dup/fs/realfs"
	"dup/lifecycle"
)

func main() {
	logFile, err := os.Create("log-dup.log")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer logFile.Close()

	log.SetOutput(logFile)
	log.SetFlags(log.Lmicroseconds)

	var lc = lifecycle.New()
	var paths []string
	var fss []fs.FS
	if len(os.Args) > 1 && (os.Args[1] == "-sim" || os.Args[1] == "-sim2") {
		fss = []fs.FS{mockfs.New("origin", lc), mockfs.New("copy 1", lc), mockfs.New("copy 2", lc)}
	} else {
		fss = make([]fs.FS, len(os.Args)-1)
		for i, path := range os.Args[1:] {
			err := os.MkdirAll(path, 0755)
			if err != nil {
				log.Printf("Failed to scan archives: %W\n", err)
				panic(err)
			}
			path, err := realfs.AbsPath(path)
			paths[i] = path
			if err != nil {
				log.Printf("Failed to scan archives: %W\n", err)
				panic(err)
			}
			fss = append(fss, realfs.New(path, lc))
		}
	}

	app.Run(fss, lc)
}
