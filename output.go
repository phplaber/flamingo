package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

func outputRst(requests []request, filepath string) {
	jsonData, err := json.Marshal(requests)
	if err != nil {
		log.Fatalf("[-] Marshal requests error: %v\n", err)
	}

	err = ioutil.WriteFile(filepath, jsonData, 0644)
	if err != nil {
		log.Fatalf("[-] Write file error: %v\n", err)
	}
}
