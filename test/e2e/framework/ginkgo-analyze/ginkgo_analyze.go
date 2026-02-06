/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	gtypes "github.com/onsi/ginkgo/v2/types"
)

func main() {
	flag.Parse()

	if flag.CommandLine.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "need exactly one Ginkgo JSON file as input")
		os.Exit(1)
	}
	input := flag.CommandLine.Arg(0)

	// TODO (?): remote download
	f, err := os.Open(input)
	if err != nil {
		panic(err)
	}

	var reports []gtypes.Report
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&reports); err != nil {
		panic(err)
	}

	if len(reports) != 1 {
		fmt.Fprintf(os.Stderr, "need exactly one report in %s\n", input)
		os.Exit(1)
	}
	report := reports[0]

	fmt.Printf(`---
displayMode: compact
---
gantt
 title %s: %s
 %% Unix milliseconds
 dateFormat x
 axisFormat %%H:%%M:%%S

`, report.StartTime.Format(time.RFC1123Z), input)

	for i, test := range report.SpecReports {
		if test.NumAttempts == 0 {
			// Never started.
			continue
		}

		// https://mermaid.js.org/syntax/gantt.html#syntax
		var tag string
		switch test.State {
		case gtypes.SpecStateSkipped:
			tag = "active"
		case gtypes.SpecStatePassed:
			tag = "done"
		default:
			tag = "crit"
		}

		fmt.Printf(" %d :%s, %d, %.3fs\n",
			i,
			tag,
			test.StartTime.UnixMilli(),
			test.EndTime.Sub(test.StartTime).Seconds(),
		)
	}
}
