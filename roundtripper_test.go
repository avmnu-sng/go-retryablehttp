// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package retryablehttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestRoundTripper_implements(t *testing.T) {
	// Compile-time proof of interface satisfaction.
	var _ http.RoundTripper = &RoundTripper{}
}

func TestRoundTripper_init(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	// Start with a new empty RoundTripper.
	rt := &RoundTripper{}

	// RoundTrip once.
	req, _ := http.NewRequest("GET", ts.URL, nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}

	// Check that the Client was initialized.
	if rt.Client == nil {
		t.Fatal("expected rt.Client to be initialized")
	}

	// Save the Client for later comparison.
	initialClient := rt.Client

	// RoundTrip again.
	req, _ = http.NewRequest("GET", ts.URL, nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}

	// Check that the underlying Client is unchanged.
	if rt.Client != initialClient {
		t.Fatalf("expected %v, got %v", initialClient, rt.Client)
	}
}

func TestRoundTripper_RoundTrip(t *testing.T) {
	var reqCount int32 = 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqNo := atomic.AddInt32(&reqCount, 1)
		if reqNo < 3 {
			w.WriteHeader(404)
		} else {
			w.WriteHeader(200)
			if _, err := w.Write([]byte("success!")); err != nil {
				t.Fatalf("failed to write: %v", err)
			}
		}
	}))
	defer ts.Close()

	// Make a client with some custom settings to verify they are used.
	retryClient := NewClient()
	retryClient.CheckRetry = func(_ context.Context, resp *http.Response, _ error) (bool, error) {
		return resp.StatusCode == 404, nil
	}

	// Get the standard client and execute the request.
	client := retryClient.StandardClient()
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Check the response to ensure the client behaved as expected.
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if v, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	} else if string(v) != "success!" {
		t.Fatalf("expected %q, got %q", "success!", v)
	}
}

func TestRoundTripper_TransportFailureErrorHandling(t *testing.T) {
	// Make a client with some custom settings to verify they are used.
	retryClient := NewClient()
	retryClient.CheckRetry = func(_ context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return true, err
		}

		return false, nil
	}

	retryClient.ErrorHandler = PassthroughErrorHandler

	expectedError := &url.Error{
		Op:  "Get",
		URL: "http://999.999.999.999:999/",
		Err: &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: &net.DNSError{
				Name:       "999.999.999.999",
				Err:        "no such host",
				IsNotFound: true,
			},
		},
	}

	// Get the standard client and execute the request.
	client := retryClient.StandardClient()
	_, err := client.Get("http://999.999.999.999:999/")

	// assert expectations
	if !reflect.DeepEqual(expectedError, normalizeError(err)) {
		t.Fatalf("expected %q, got %q", expectedError, err)
	}
}

func normalizeError(err error) error {
	var dnsError *net.DNSError

	if errors.As(err, &dnsError) {
		// this field is populated with the DNS server on on CI, but not locally
		dnsError.Server = ""
	}

	return err
}
