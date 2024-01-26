package probing

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestHTTPCaller_RunWithContext(t *testing.T) {
	caller := NewHttpCaller("https://google.com",
		WithHTTPCallerTargetRPS(5),
		WithHTTPCallerMaxConcurrentCalls(2),
		WithHTTPCallerOnGotFirstByte(func(suite *TraceSuite) {
			fmt.Println(suite.GetFirstByteReceived().Sub(suite.GetRequestWritten()))
		}),
	)

	ctx := context.Background()
	go caller.RunWithContext(ctx)
	time.Sleep(time.Second * 5)
	ctx.Done()
}
