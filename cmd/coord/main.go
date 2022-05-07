package main

import (
	coord "STor/coord"
	"fmt"
)

func main() {
	c, err := coord.NewCoord("config/coord_config.json")
	if err != nil {
		fmt.Println("Received an error:", err)
		return
	}
	fmt.Println(c)

	err = c.StartCoord()
	fmt.Println("Encountered an error:", err)
}
