package main

import (
	"encoding/json"
	"log"
	"os"
)

func outputRst(requests []request, filepath string) {
	file, err := os.Create(filepath)
	if err != nil {
		log.Fatalf("[-] Create file error: %v\n", err)
	}
	defer file.Close()
	
	// 使用流式编码，减少内存占用
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(requests); err != nil {
		log.Fatalf("[-] Encode requests error: %v\n", err)
	}
}
