package util

import (
	storprotocol "STor/interface"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

func ReadJSONConfig(filename string, config interface{}) error {
	configData, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	err = json.Unmarshal(configData, config)
	if err != nil {
		return err
	}
	return nil
}

func CheckErr(err error, errfmsg string, fargs ...interface{}) {
	if err != nil {
		fmt.Fprintf(os.Stderr, errfmsg, fargs...)
		os.Exit(1)
	}
}

func RouterIds(routers []storprotocol.Router) []int {
	var routerIds []int
	for _, x := range routers {
		routerIds = append(routerIds, x.RouterId)
	}
	return routerIds
}
