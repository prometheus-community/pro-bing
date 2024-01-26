package probing

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWithHTTPCallerClient(t *testing.T) {

}

func TestWithHTTPCallerTargetRPS(t *testing.T) {

}

func TestWithHTTPCallerMaxConcurrentCalls(t *testing.T) {

}

func TestWithHTTPCallerHeaders(t *testing.T) {

}

func TestWithHTTPCallerMethod(t *testing.T) {

}

func TestWithHTTPCallerBody(t *testing.T) {

}

func TestWithHTTPCallerTimeout(t *testing.T) {

}

func TestWithHTTPCallerIsValidResponse(t *testing.T) {

}

func TestWithHTTPCallerOnResp(t *testing.T) {

}

func TestWithHTTPCallerOnFinish(t *testing.T) {

}

func TestWithHTTPCallerLogger(t *testing.T) {

}

func TestHTTPCaller_RunWithContext(t *testing.T) {
	caller := NewHttpCaller("https://google.com",
		WithHTTPCallerTargetRPS(5),
		WithHTTPCallerMaxConcurrentCalls(2),
	)

	ctx := context.Background()
	go func() {
		err := caller.RunWithContext(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(time.Second * 5)
	ctx.Done()
	fmt.Println(caller.Statistics())
}
