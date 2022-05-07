package main

import (
	"STor/router"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please include router number")
		os.Exit(1)
	}
	routerId := os.Args[1]
	r := router.NewRouter(fmt.Sprintf("config/router_config%s.json", routerId))
	r.StartRouter()
}
