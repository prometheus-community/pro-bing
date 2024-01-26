package probing

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestHTTPCaller_RunWithContext(t *testing.T) {
	caller := NewHttpCaller("https://google.com", http.MethodGet, nil)
	caller.SetTargetRPS(5)
	caller.SetMaxConcurrentCalls(2)

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
