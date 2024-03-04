/*
Copyright 2024 The Kubernetes Authors.

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

package restproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"

	"github.com/go-logr/logr"

	"k8s.io/klog/v2"
	drapb "k8s.io/kubelet/pkg/apis/dra/v1alpha3"
)

// StartClient creates a new client. Canceling the context
// stops all background activity.
func StartClient(ctx context.Context) *Client {
	logger := klog.FromContext(ctx)
	logger = klog.LoggerWithName(logger, "REST Client")
	ctx = klog.NewContext(ctx, logger)
	c := &Client{ctx: ctx}
	c.cond = sync.NewCond(&c.mutex)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		<-ctx.Done()

		// Notify goroutines which are blocked on c.cond.Wait.
		// All goroutines must check for cancellation before
		// calling Wait.
		c.mutex.Lock()
		defer c.mutex.Unlock()
		c.cond.Broadcast()
	}()
	return c
}

var _ http.RoundTripper = &Client{}

// Client is sending REST requests to the server via the proxy.
// It implements [http.RoundTripper].
type Client struct {
	ctx   context.Context
	mutex sync.Mutex
	cond  *sync.Cond
	wg    sync.WaitGroup

	// idCounter continuously gets incremented for each new request.
	idCounter int64

	// proxyServer is nil if we don't have an active stream to a proxy.
	proxyServer drapb.Node_NodeStartRESTProxyServer

	// streams contains all active streams, indexed by their ID.
	streams map[int64]*stream
}

type stream struct {
	id int64

	// responseHeader is nil while we have no response yet.
	responseHeader *drapb.RESTResponseHeader

	// responseData is all data received so far, sorted by offset.
	responseData []chunk

	// read is the total amount of data consumed from the response data.
	// There's no guarantee that chunks get stored in order, so [bodyReader.Read]
	// has to check that the next data is what it needs.
	read int64

	// errMsg is the error string that shall be returned by [bodyReader.Read]
	// after delivering the response data.
	errMsg string

	// closeAt indicates that no more data is coming after consuming that
	// amount of bytes. -1 when unknown.
	closeAt int64
}

// MarshalLog logs the struct without any pointers and without data.
func (s *stream) MarshalLog() any {
	obj := map[string]any{
		"id": s.id,
		"responseOffsets": func() []int64 {
			offsets := make([]int64, len(s.responseData))
			for i := range s.responseData {
				offsets[i] = s.responseData[i].offset
			}
			return offsets
		}(),
		"read":    s.read,
		"errMsg":  s.errMsg,
		"closeAt": s.closeAt,
	}
	return obj
}

var _ logr.Marshaler = &stream{}

type chunk struct {
	offset int64
	data   []byte
}

// WaitForShutdown blocks until all goroutines have stopped running.
// The context of the client must get canceled separately.
func (c *Client) WaitForShutdown() {
	c.wg.Wait()
}

// NodeStartRESTProxy stores the server stream and thus enables sending REST
// requests to the proxy through that stream.
func (c *Client) NodeStartRESTProxy(req *drapb.NodeStartRESTProxyMessage, server drapb.Node_NodeStartRESTProxyServer) error {
	logger := klog.FromContext(c.ctx)

	func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		if c.proxyServer != nil {
			// Called again? Close all previous requests.
			c.streams = nil
		}

		c.proxyServer = server
		c.streams = make(map[int64]*stream)
		c.cond.Broadcast()
		logger.V(2).Info("Started client of REST proxy")
	}()

	// We need to block until it is time to shut down because returning
	// would close the stream from our end.
	<-c.ctx.Done()
	logger.V(2).Info("Stopped client of REST proxy")

	return nil
}

// NodeRESTReply receives more information about a request and updates the
// response stream with it. It doesn't block.
func (c *Client) NodeRESTReply(ctx context.Context, req *drapb.NodeRESTReplyRequest) (*drapb.NodeRESTReplyResponse, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	stream := c.streams[req.Id]

	logger := klog.FromContext(ctx)
	if stream == nil {
		logger.V(2).Info("Closing REST proxy response stream, it is gone", "requestID", req.Id)
		return &drapb.NodeRESTReplyResponse{
			Close: true,
		}, nil
	}

	if stream.responseHeader == nil {
		header := *req.Header
		stream.responseHeader = &header
	}
	if req.Error != "" {
		stream.errMsg = req.Error
	}
	if req.CloseAt >= 0 {
		stream.closeAt = req.CloseAt
	}
	// Insert sorted by offset, lowest offset first.
	i := stream.nextEntry(req.BodyOffset)
	slices.Insert(stream.responseData, i, chunk{offset: req.BodyOffset, data: req.Body})
	logger.V(5).Info("REST response stream updated", "steam", stream)

	c.cond.Broadcast()

	return nil, nil
}

// nextEntry returns the index of the first entry
// which has a higher offset than the given one, or the length
// of the responseData slice if there is none.
func (r *stream) nextEntry(offset int64) int {
	for i := range r.responseData {
		if r.responseData[i].offset > offset {
			return i
		}
	}
	return len(r.responseData)
}

// RoundTrip implements [http.RoundTripper.Roundtrip].
func (c *Client) RoundTrip(req *http.Request) (finalResp *http.Response, finalErr error) {
	id := int64(0)
	c.mutex.Lock()
	defer func() {
		defer c.mutex.Unlock()

		// Always remove the stream when we are not going to read from it.
		if finalErr != nil {
			delete(c.streams, id)
			c.cond.Broadcast()
		}
	}()

	// Generate new stream.
	c.idCounter++
	id = c.idCounter
	stream := &stream{
		id: id,
	}
	c.streams[id] = stream
	klog.V(3).Info("New REST request", "id", id)

	// Determine whether the request has a context or cancel stream.
	// A goroutine will block on those and notify the parent when
	// cancellation happened.
	reqDone := req.Cancel
	canceled := false // protected by c.mutex
	if reqCtx := req.Context(); reqCtx != nil {
		reqDone = reqCtx.Done()
	}
	if reqDone != nil {
		go func() {
			<-reqDone

			c.mutex.Lock()
			defer c.mutex.Unlock()
			canceled = true
			klog.V(3).Info("REST request cancelled by client", "id", id)
		}()
	}

	// Wait for proxy.
	for c.proxyServer == nil {
		if err := context.Cause(c.ctx); err != nil {
			return nil, fmt.Errorf("waiting for proxy server failed: %w", err)
		}
		c.cond.Wait()
	}

	// Tell the proxy to start the request.
	//
	// It is not safe to call Send on the same stream in different
	// goroutines, so we have to hold this lock while sending.
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}
	req.Body.Close()
	proxyHeader := make(map[string]*drapb.RESTHeader, len(req.Header))
	for name, values := range req.Header {
		proxyHeader[name] = &drapb.RESTHeader{Values: values}
	}
	proxyReq := &drapb.NodeRESTProxyRequest{
		Id:     stream.id,
		Method: req.Method,
		Path:   req.URL.Path,
		Header: proxyHeader,
		Body:   body,
	}
	if err := c.proxyServer.Send(proxyReq); err != nil {
		return nil, fmt.Errorf("sending proxy request: %w", err)
	}

	// Response information and data will be received by NodeRESTReply
	// and gets stored in the stream. From there the body reader takes
	// as much data as it can until the stream is closed or failed.
	// But first we need the initial response.
	for {
		stream := c.streams[id]
		if stream == nil {
			// Stream got removed.
			return nil, errors.New("stream got removed")
		}

		if stream.responseHeader != nil {
			header := make(http.Header, len(stream.responseHeader.Header))
			for name, values := range stream.responseHeader.Header {
				header[name] = values.Values
			}
			req.Body = nil
			resp := &http.Response{
				Status:     stream.responseHeader.Status,
				StatusCode: int(stream.responseHeader.StatusCode),
				Proto:      stream.responseHeader.Proto,
				ProtoMajor: int(stream.responseHeader.ProtoMajor),
				ProtoMinor: int(stream.responseHeader.ProtoMinor),
				Header:     header,
				Body: bodyReader{
					client: c,
					id:     id,
				},
				Request: req,
			}
			return resp, nil
		}

		if canceled {
			return nil, fmt.Errorf("REST request canceled by client: %w", context.Canceled)
		}
		if err := context.Cause(c.ctx); err != nil {
			return nil, fmt.Errorf("REST proxy canceled: %w", err)
		}
		c.cond.Wait()
	}
}

// bodyReader holds a reference to the client and reads response data for one
// stream.
type bodyReader struct {
	client *Client
	id     int64
}

func (b bodyReader) Read(p []byte) (int, error) {
	c := b.client
	logger := klog.FromContext(c.ctx)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for {
		stream := c.streams[b.id]
		if stream == nil {
			// Stream got removed.
			return 0, errors.New("stream got removed")
		}

		if stream.read == stream.closeAt {
			// All data consumed without an error.
			return 0, io.EOF
		}

		if len(stream.responseData) > 0 {
			head := stream.responseData[0]
			if head.offset <= stream.read {
				// start must be inside the first chunk, otherwise it would
				// have been removed already.
				start := int(stream.read - head.offset)
				remaining := len(head.data) - start
				n := len(p)
				if n > remaining {
					n = remaining
				}
				copy(p, head.data[start:start+n])
				if start+n == len(head.data) {
					// Done with the first chunk.
					stream.responseData[0].data = nil
					slices.Delete(stream.responseData, 0, 1)
				}
				stream.read += int64(n)
				logger.V(3).Info("Updated stream after reading from response", "n", n, "stream", stream)
				return n, nil
			}
		}

		if stream.errMsg != "" {
			return 0, errors.New(stream.errMsg)
		}

		// TODO: should this support cancellation via the request context?

		if err := context.Cause(c.ctx); err != nil {
			return 0, fmt.Errorf("REST proxy canceled: %w", err)
		}

		c.cond.Wait()
	}
}

// Close removes the stream, which indicates to the proxy that no further is
// needed if it tries to add more.
func (b bodyReader) Close() error {
	c := b.client
	logger := klog.FromContext(c.ctx)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.streams, b.id)
	logger.V(3).Info("Closing REST stream because response body got closed", "id", b.id)
	c.cond.Broadcast()
	return nil
}
