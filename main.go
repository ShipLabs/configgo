package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

// basic file loading
func LoadSingleFile(path string) (config map[string]string, err error) {
	config = make(map[string]string)
	//for now handle .env files alone (other file extensions to consider inclue )
	if path == "" {
		path = ".env"
	}
	file, err := os.Open(path)
	if err != nil {
		//you may want to exit the whole program with an error?? think log.fatal??
		return //how would I better handle the error depening on what it may be????
	}
	defer file.Close()

	scannner := bufio.NewScanner(file)
	for scannner.Scan() {
		skip, key, val := parseLine(scannner.Text())
		if skip {
			continue
		}
		fmt.Println(key, val)
		config[key] = val
	}

	return
}

func parseLine(line string) (skip bool, key string, val string) {
	skip = true
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	const INVALID_LINE_PREFIXES = "//!@#;:=[]"
	for _, prefix := range INVALID_LINE_PREFIXES {
		if strings.HasPrefix(line, string(prefix)) {
			return
		}
	}

	//test this againts edge cases(":=")
	keyVal := strings.FieldsFunc(line, func(r rune) bool {
		return r == '=' || r == ':'
	})
	if len(keyVal) != 2 {
		return
	}

	//checks if the line contains section markers like: [section]
	if strings.HasPrefix(keyVal[0], "[") && strings.HasSuffix(keyVal[1], "]") {
		return
	}

	return false, keyVal[0], keyVal[1]

}

// for testing
func main() {
	config, err := LoadSingleFile("")
	if err != nil {
		log.Fatalln(err)
		return
	}
	fmt.Println(config)
}
