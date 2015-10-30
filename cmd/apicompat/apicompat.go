package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/rogpeppe/apicompat"
	"github.com/rogpeppe/apicompat/jsontypes"
	"io/ioutil"
	"log"
)

func main() {
	flag.Parse()
	if flag.NArg() != 2 {
		log.Fatal("usage: check api_old.json api_new.json")
	}
	info0, err := readInfo(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	info1, err := readInfo(flag.Arg(1))
	if err != nil {
		log.Fatal(err)
	}
	for _, t0 := range info0.Types {
		t1, ok := info1.Types[t0.Name]
		if !ok {
			fmt.Printf("type %s has gone away", t0.Name)
			continue
		}
		err := apicompat.Check(info0, info1, t0, t1, customMarshaler)
		if err != nil {
			err := err.(*apicompat.CheckError)
			for _, err := range err.Errors {
				fmt.Printf("%s incompatible: %v", t0.Name, err)
			}
		}
	}
}

func readInfo(f string) (*jsontypes.Info, error) {
	data, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, err
	}
	var info *jsontypes.Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	// Remove all non-marshaling-related methods
	// because they're irrelevant to our compatiblity.
	apicompat.PruneMethods(info, func(t *jsontypes.Type, m *jsontypes.Method) bool {
		for _, name := range marshalMethodNames {
			if m.Name == name {
				return true
			}
		}
		return false
	})
	return info, nil
}

var marshalMethodNames = []string{
	"MarshalJSON",
	"UnmarshalJSON",
	"MarshalText",
	"UnmarshalText",
}

func customMarshaler(info *jsontypes.Info, t *jsontypes.Type) bool {
	for _, name := range marshalMethodNames {
		if t.Methods[name] != nil {
			// TODO check sig too
			return true
		}
	}
	return false
}
