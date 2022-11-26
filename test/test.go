package test

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func RunCommand(s string) string {
	path, _ := filepath.Abs("./worker.exe")
	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	var result string
	go func() {
		in := bufio.NewReader(stdout)
		for {
			sd, err := in.ReadString('\n')
			if err != nil {
				return
			}
			result = sd
		}
	}()
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	g, _ := io.WriteString(stdin, s)
	fmt.Println((g))
	if err := cmd.Wait(); err != nil {
		panic(err)
	}

	return result
}
