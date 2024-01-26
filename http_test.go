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
	fmt.Println(caller.statusCodesCount)
}
