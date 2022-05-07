package main

import (
	"STor/client"
	"fmt"
	"os"
)

func main() {

	if len(os.Args) < 2 {
		fmt.Println("Please include client number")
		os.Exit(1)
	}
	clientNum := os.Args[1]
	client.Init(clientNum)
}
