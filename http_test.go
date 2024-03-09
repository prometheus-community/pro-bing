package probing

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"sync"
	"testing"
	"time"
)

// TODO: figure out how to test onDNS callback
func getTestHTTPClientServer() (*http.Client, *httptest.Server) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	return srv.Client(), srv
}

func TestHTTPCaller_MakeCall_OnReq(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnReq(func(suite *TraceSuite) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeZero(t, suite.generalEnd)
			AssertTimeZero(t, suite.connStart)
			AssertTimeZero(t, suite.connEnd)
			AssertTimeZero(t, suite.tlsStart)
			AssertTimeZero(t, suite.tlsEnd)
			AssertTimeZero(t, suite.wroteHeaders)
			AssertTimeZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_OnConnStart(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnConnStart(func(suite *TraceSuite, network, addr string) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeZero(t, suite.generalEnd)
			AssertTimeNonZero(t, suite.connStart)
			AssertTimeZero(t, suite.connEnd)
			AssertTimeZero(t, suite.tlsStart)
			AssertTimeZero(t, suite.tlsEnd)
			AssertTimeZero(t, suite.wroteHeaders)
			AssertTimeZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_OnConnDone(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnConnDone(func(suite *TraceSuite, network, addr string, err error) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeZero(t, suite.generalEnd)
			AssertTimeNonZero(t, suite.connStart)
			AssertTimeNonZero(t, suite.connEnd)
			AssertTimeZero(t, suite.tlsStart)
			AssertTimeZero(t, suite.tlsEnd)
			AssertTimeZero(t, suite.wroteHeaders)
			AssertTimeZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_OnTLSStart(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnTLSStart(func(suite *TraceSuite) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeZero(t, suite.generalEnd)
			AssertTimeNonZero(t, suite.connStart)
			AssertTimeNonZero(t, suite.connEnd)
			AssertTimeNonZero(t, suite.tlsStart)
			AssertTimeZero(t, suite.tlsEnd)
			AssertTimeZero(t, suite.wroteHeaders)
			AssertTimeZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_OnTLSDone(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnTLSDone(func(suite *TraceSuite, state tls.ConnectionState, err error) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeZero(t, suite.generalEnd)
			AssertTimeNonZero(t, suite.connStart)
			AssertTimeNonZero(t, suite.connEnd)
			AssertTimeNonZero(t, suite.tlsStart)
			AssertTimeNonZero(t, suite.tlsEnd)
			AssertTimeZero(t, suite.wroteHeaders)
			AssertTimeZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_OnWroteHeaders(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnWroteRequest(func(suite *TraceSuite) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeZero(t, suite.generalEnd)
			AssertTimeNonZero(t, suite.connStart)
			AssertTimeNonZero(t, suite.connEnd)
			AssertTimeNonZero(t, suite.tlsStart)
			AssertTimeNonZero(t, suite.tlsEnd)
			AssertTimeNonZero(t, suite.wroteHeaders)
			AssertTimeZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_OnFirstByteReceived(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnFirstByteReceived(func(suite *TraceSuite) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeZero(t, suite.generalEnd)
			AssertTimeNonZero(t, suite.connStart)
			AssertTimeNonZero(t, suite.connEnd)
			AssertTimeNonZero(t, suite.tlsStart)
			AssertTimeNonZero(t, suite.tlsEnd)
			AssertTimeNonZero(t, suite.wroteHeaders)
			AssertTimeNonZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_OnResp(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callbackCalled bool
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerClient(client),
		WithHTTPCallerOnResp(func(suite *TraceSuite, info *HTTPCallInfo) {
			AssertTimeNonZero(t, suite.generalStart)
			AssertTimeNonZero(t, suite.generalEnd)
			AssertTimeNonZero(t, suite.connStart)
			AssertTimeNonZero(t, suite.connEnd)
			AssertTimeNonZero(t, suite.tlsStart)
			AssertTimeNonZero(t, suite.tlsEnd)
			AssertTimeNonZero(t, suite.wroteHeaders)
			AssertTimeNonZero(t, suite.firstByteReceived)
			callbackCalled = true
		}))
	err := httpCaller.makeCall(context.Background())
	AssertNoError(t, err)
	AssertTrue(t, callbackCalled)
}

func TestHTTPCaller_MakeCall_IsValidResponse(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	t.Run("no callback", func(t *testing.T) {
		var callbackCalled bool
		httpCaller := NewHttpCaller(srv.URL,
			WithHTTPCallerClient(client),
			WithHTTPCallerOnResp(func(suite *TraceSuite, info *HTTPCallInfo) {
				AssertTrue(t, info.IsValidResponse)
				callbackCalled = true
			}),
		)
		err := httpCaller.makeCall(context.Background())
		AssertNoError(t, err)
		AssertTrue(t, callbackCalled)
	})

	t.Run("false callback", func(t *testing.T) {
		var callbackCalled bool
		httpCaller := NewHttpCaller(srv.URL,
			WithHTTPCallerClient(client),
			WithHTTPCallerIsValidResponse(func(response *http.Response, body []byte) bool {
				return false
			}),
			WithHTTPCallerOnResp(func(suite *TraceSuite, info *HTTPCallInfo) {
				AssertFalse(t, info.IsValidResponse)
				callbackCalled = true
			}),
		)
		err := httpCaller.makeCall(context.Background())
		AssertNoError(t, err)
		AssertTrue(t, callbackCalled)
	})

	t.Run("true callback", func(t *testing.T) {
		var callbackCalled bool
		httpCaller := NewHttpCaller(srv.URL,
			WithHTTPCallerClient(client),
			WithHTTPCallerIsValidResponse(func(response *http.Response, body []byte) bool {
				return true
			}),
			WithHTTPCallerOnResp(func(suite *TraceSuite, info *HTTPCallInfo) {
				AssertTrue(t, info.IsValidResponse)
				callbackCalled = true
			}),
		)
		err := httpCaller.makeCall(context.Background())
		AssertNoError(t, err)
		AssertTrue(t, callbackCalled)
	})
}

func TestHTTPCaller_RunWithContext(t *testing.T) {
	client, srv := getTestHTTPClientServer()
	defer srv.Close()

	var callsCount int
	var callsCountMu sync.Mutex
	httpCaller := NewHttpCaller(srv.URL,
		WithHTTPCallerMaxConcurrentCalls(5),
		WithHTTPCallerCallFrequency(time.Second/5),
		WithHTTPCallerClient(client),
		WithHTTPCallerTimeout(time.Second),
		WithHTTPCallerOnReq(func(suite *TraceSuite) {
			callsCountMu.Lock()
			defer callsCountMu.Unlock()
			callsCount++
		}),
	)
	ctx := context.Background()
	go func() {
		httpCaller.RunWithContext(ctx)
	}()
	time.Sleep(time.Second)
	done := make(chan struct{})
	go func() {
		ctx.Done()
		done <- struct{}{}
	}()
	select {
	case <-time.After(time.Second * 2):
		t.Errorf("Timed out on a shutdown, meaning workload wasn't processed, Stack: \n%s", string(debug.Stack()))
	case <-done:
	}
	httpCaller.Stop()
	AssertIntGreaterOrEqual(t, callsCount, 5)
}

func AssertTimeZero(t *testing.T, tm time.Time) {
	t.Helper()
	if !tm.IsZero() {
		t.Errorf("Expected zero time, got non zero time, Stack: \n%s", string(debug.Stack()))
	}
}

func AssertTimeNonZero(t *testing.T, tm time.Time) {
	t.Helper()
	if tm.IsZero() {
		t.Errorf("Expected non zero time, got zero time, Stack: \n%s", string(debug.Stack()))
	}
}

func AssertIntGreaterOrEqual(t *testing.T, expected, actual int) {
	t.Helper()
	if actual < expected {
		t.Errorf("Exptected value to be less then %v, got %v, Stack: \n%s", expected, actual, string(debug.Stack()))
	}
}
