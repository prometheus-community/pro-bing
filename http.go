package probing

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultHTTPTargetRPS          = 1
	defaultHTTPMaxConcurrentCalls = 10
	defaultTimeout                = time.Second * 10
)

func NewHttpCaller(url string, method string, body []byte) *HTTPCaller {
	return &HTTPCaller{
		TargetRPS:          defaultHTTPTargetRPS,          // TODO: describe this default
		MaxConcurrentCalls: defaultHTTPMaxConcurrentCalls, // TODO: describe this default
		URL:                url,
		Method:             method,
		Body:               body,
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
	Body    []byte
	Timeout time.Duration

	IsValidResponse func(response *http.Response, body []byte) bool // TODO: annotate

	client http.Client // TODO: allow client interface

	statsMu             sync.Mutex
	statusCodesCount    map[int]int
	callsCount          int
	validResponsesCount int
	minLatency          time.Duration
	maxLatency          time.Duration
	avgLatency          time.Duration
	stdDevM2Latency     time.Duration
	stdDevLatency       time.Duration

	workChan chan struct{}
	doneChan chan struct{}
	doneWg   sync.WaitGroup

	logger Logger

	OnResp   func(*HTTPCallInfo)
	OnFinish func(*HTTPStatistics)
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
	if c.OnFinish != nil {
		c.OnFinish(c.Statistics())
	}
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
			case <-c.doneChan:
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
				case <-c.doneChan:
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

	req, err := http.NewRequestWithContext(ctx, c.Method, c.URL, bytes.NewReader(c.Body))
	if err != nil {
		return err // TODO: wrap
	}
	for header, value := range c.Headers {
		req.Header.Add(header, value)
	}

	requestTime := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body) // TODO: ok? think about it
	if err != nil {
		return err // TODO: wrap
	}
	resp.Body.Close() // TODO: err?
	responseTime := time.Now()
	latency := responseTime.Sub(requestTime)
	isValidResponse := true
	if c.IsValidResponse != nil {
		isValidResponse = c.IsValidResponse(resp, body)
	}

	// TODO: channels?
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.statusCodesCount[resp.StatusCode]++
	c.callsCount++
	if isValidResponse {
		c.validResponsesCount++
	}
	if c.callsCount == 1 || latency < c.minLatency {
		c.minLatency = latency
	}
	if latency > c.maxLatency {
		c.maxLatency = latency
	}
	c.stdDevLatency, c.stdDevM2Latency, c.avgLatency = calculateStdDev(c.callsCount, latency, c.avgLatency, c.stdDevM2Latency)

	if c.OnResp != nil {
		c.OnResp(&HTTPCallInfo{
			RequestTime:     requestTime,
			ResponseTime:    responseTime,
			StatusCode:      resp.StatusCode,
			IsValidResponse: isValidResponse,
		})
	}
	return nil
}

func (c *HTTPCaller) Statistics() *HTTPStatistics {
	c.statsMu.Lock() // TODO: rwMu?
	defer c.statsMu.Unlock()

	statusCodesCount := make(map[int]int, len(c.statusCodesCount))
	for k, v := range c.statusCodesCount {
		statusCodesCount[k] = v
	}
	return &HTTPStatistics{
		StatusCodesCount:    statusCodesCount,
		CallsCount:          c.callsCount,
		ValidResponsesCount: c.validResponsesCount,
		MinLatency:          c.minLatency,
		MaxLatency:          c.maxLatency,
		AvgLatency:          c.avgLatency,
		StdDevLatency:       c.stdDevLatency,
	}
}

// TODO: do we want to provide a resopnse? Think about it
type HTTPCallInfo struct {
	RequestTime     time.Time
	ResponseTime    time.Time
	StatusCode      int
	IsValidResponse bool // TODO: think about the naming here, ANNOTATE!
}

// TODO: annotate all fields
type HTTPStatistics struct {
	StatusCodesCount    map[int]int
	CallsCount          int
	ValidResponsesCount int
	MinLatency          time.Duration
	MaxLatency          time.Duration
	AvgLatency          time.Duration
	StdDevLatency       time.Duration
}
