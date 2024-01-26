package probing

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const (
	defaultHTTPTargetRPS          = 1
	defaultHTTPMaxConcurrentCalls = 10
	defaultTimeout                = time.Second * 10
)

func NewHttpCaller(url string, method string) *HTTPCaller {
	return &HTTPCaller{
		TargetRPS:          defaultHTTPTargetRPS,          // TODO: describe this default
		MaxConcurrentCalls: defaultHTTPMaxConcurrentCalls, // TODO: describe this default
		URL:                url,
		Method:             method,
		Timeout:            defaultTimeout, // TODO: describe this default

		workChan: make(chan struct{}, defaultHTTPMaxConcurrentCalls),

		statusCodesCount: make(map[int]int),
	}
}

type HTTPCaller struct {
	TargetRPS          int
	MaxConcurrentCalls int

	URL     string
	Headers map[string]string
	Method  string
	Timeout time.Duration

	// TODO: valid Headers check
	// TODO: valid Body check

	client http.Client // TODO: allow client interface

	statsMu          sync.Mutex
	statusCodesCount map[int]int
	callsCount       int
	minLatency       time.Duration
	maxLatency       time.Duration
	avgLatency       time.Duration
	stdDevLatency    time.Duration

	workChan chan struct{}
	doneChan chan struct{}
	doneWg   sync.WaitGroup

	logger Logger
}

func (c *HTTPCaller) SetTimeout(timeout time.Duration) {
	c.Timeout = timeout
}

func (c *HTTPCaller) SetTargetRPS(rps int) {
	c.TargetRPS = rps
}

// TODO: annotate the behaviour about the channel
func (c *HTTPCaller) SetMaxConcurrentCalls(num int) {
	c.MaxConcurrentCalls = num
	c.workChan = make(chan struct{}, num)
}

func (c *HTTPCaller) SetLogger(logger Logger) {
	c.logger = logger
}

func (c *HTTPCaller) Stop() {
	close(c.doneChan)
	c.doneWg.Wait()
}

// TODO: think about checks for overflow etc
func (c *HTTPCaller) RunWithContext(ctx context.Context) error {
	c.runWorkCreator(ctx)
	c.runCallers(ctx)
	c.doneWg.Wait()
	return nil
}

// TODO: rename
func (c *HTTPCaller) runWorkCreator(ctx context.Context) {
	c.doneWg.Add(1)
	go func() {
		defer c.doneWg.Done()

		freq := int(time.Second) / c.TargetRPS // TODO: explanation comment
		// TODO: move to separate function, freq calc & TEST

		ticker := time.NewTicker(time.Duration(freq))
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.workChan <- struct{}{}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (c *HTTPCaller) runCallers(ctx context.Context) {
	for i := 0; i < c.MaxConcurrentCalls; i++ {
		c.doneWg.Add(1)
		go func() {
			defer c.doneWg.Done()
			for {
				logger := c.logger
				if logger == nil {
					logger = NoopLogger{}
				}
				select {
				case <-c.workChan:
					if err := c.makeCall(ctx); err != nil {
						logger.Errorf("failed making a call: %v", err)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}

// TODO: check http client effective lifehacks
func (c *HTTPCaller) makeCall(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, c.Method, c.URL, nil) // TODO: support req body
	if err != nil {
		return err // TODO: wrap
	}
	for header, value := range c.Headers {
		req.Header.Add(header, value)
	}

	callStart := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close() // TODO: err?
	latency := time.Now().Sub(callStart)

	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.statusCodesCount[resp.StatusCode]++
	c.callsCount++
	if c.callsCount == 1 || latency < c.minLatency {
		c.minLatency = latency
	}
	if latency > c.maxLatency {
		c.maxLatency = latency
	}
	// TODO: calc avg & stddev

	return nil
}
