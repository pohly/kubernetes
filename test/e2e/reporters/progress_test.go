/*
Copyright 2022 The Kubernetes Authors.

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

// This test uses etcd that is only fully supported for AMD64 and Linux
// https://etcd.io/docs/v3.5/op-guide/supported-platform/#support-tiers

package reporters_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/onsi/ginkgo/v2"

	e2ereporters "k8s.io/kubernetes/test/e2e/reporters"
)

// This is a Ginkgo test suite with some tests that pass, some that get skipped,
// and some that fail.

var _ = ginkgo.Describe("e2e", func() {
	ginkgo.It("works", func() {
	})

	ginkgo.It("fails", func() {
		ginkgo.Fail("fake problem")
	})

	ginkgo.It("skips", func() {
		ginkgo.Skip("fake skip reason")
	})

	// This test gets excluded by Ginkgo itself, which makes the total
	// number of tests to run smaller than the total number of tests.
	ginkgo.XIt("pending", func() {
	})
})

func TestCleanup(t *testing.T) {
	// Run a simple HTTP server.
	var reports []e2ereporters.ProgressReporter
	http.HandleFunc("/progress", func(resp http.ResponseWriter, req *http.Request) {
		data, err := ioutil.ReadAll(req.Body)
		if err != nil {
			t.Errorf("Unexpected error reading HTTP request body: %v", err)
			return
		}
		t.Logf("HTTP %s request with body:\n%s", req.Method, string(data))
		var report e2ereporters.ProgressReporter
		if err := json.Unmarshal(data, &report); err != nil {
			t.Errorf("Unexpected error unmarshalling HTTP %s request body: %v\n    %s", req.Method, err, string(data))
		}
		reports = append(reports, report)
	})
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Could not listen on random localhost port: %v", err)
	}
	var server http.Server
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Serve(l)
	}()

	url := fmt.Sprintf("http://localhost:%d/progress", l.Addr().(*net.TCPAddr).Port)

	// The following code simulates how test/e2e uses the framework, the
	// progress reporter and how users invoke test/e2e.
	var progressReporter = &e2ereporters.ProgressReporter{}
	ginkgo.SynchronizedBeforeSuite(func(ctx context.Context) []byte {
		progressReporter.SetStartMsg()
		return nil
	}, func(ctx context.Context, data []byte) {
	})

	ginkgo.SynchronizedAfterSuite(func() {
		progressReporter.SetEndMsg()
	}, func(ctx context.Context) {
	})

	ginkgo.ReportBeforeSuite(func(report ginkgo.Report) {
		progressReporter.SetTestsTotal(report.PreRunStats.SpecsThatWillRun)
	})

	ginkgo.ReportAfterEach(func(report ginkgo.SpecReport) {
		progressReporter.ProcessSpecReport(report)
	})

	progressReporter = e2ereporters.NewProgressReporter(url)

	fakeT := &testing.T{}
	ginkgo.RunSpecs(fakeT, "Report Suite")

	// Ensure that all requests were handled.
	progressReporter.Wait()
	server.Shutdown(context.Background())
	wg.Wait()

	// TODO: validate reports
	t.Logf("Got reports: %+v", reports)
}
