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
	defaultHTTPMethod             = http.MethodGet
	defaultTimeout                = time.Second * 10
)

type httpCallerOptions struct {
	client http.Client

	targetRPS          int
	maxConcurrentCalls int

	headers http.Header
	method  string
	body    []byte
	timeout time.Duration

	isValidResponse func(response *http.Response, body []byte) bool

	onResp   func(*HTTPCallInfo)
	onFinish func(*HTTPStatistics)

	logger Logger
}

type HTTPCallerOption func(options *httpCallerOptions)

// TODO: ptr input?
func WithHTTPCallerClient(client http.Client) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.client = client
	}
}

func WithHTTPCallerTargetRPS(rps int) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.targetRPS = rps
	}
}

func WithHTTPCallerMaxConcurrentCalls(max int) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.maxConcurrentCalls = max
	}
}

func WithHTTPCallerHeaders(headers http.Header) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.headers = headers
	}
}

func WithHTTPCallerMethod(method string) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.method = method
	}
}

func WithHTTPCallerBody(body []byte) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.body = body
	}
}

func WithHTTPCallerTimeout(timeout time.Duration) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.timeout = timeout
	}
}

func WithHTTPCallerIsValidResponse(isValid func(response *http.Response, body []byte) bool) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.isValidResponse = isValid
	}
}

func WithHTTPCallerOnResp(onResp func(*HTTPCallInfo)) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.onResp = onResp
	}
}

func WithHTTPCallerOnFinish(onFinish func(statistics *HTTPStatistics)) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.onFinish = onFinish
	}
}

func WithHTTPCallerLogger(logger Logger) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.logger = logger
	}
}

func NewHttpCaller(url string, options ...HTTPCallerOption) *HTTPCaller {
	opts := httpCallerOptions{
		targetRPS:          defaultHTTPTargetRPS,
		maxConcurrentCalls: defaultHTTPMaxConcurrentCalls,
		method:             defaultHTTPMethod,
		timeout:            defaultTimeout,
	}
	for _, opt := range options {
		opt(&opts)
	}

	return &HTTPCaller{
		client: opts.client,

		targetRPS:          opts.targetRPS,
		maxConcurrentCalls: opts.maxConcurrentCalls,

		url:     url,
		headers: opts.headers,
		method:  opts.method,
		body:    opts.body,
		timeout: opts.timeout,

		isValidResponse: opts.isValidResponse,

		statusCodesCount: make(map[int]int),

		workChan: make(chan struct{}, defaultHTTPMaxConcurrentCalls),
		doneChan: make(chan struct{}),

		onResp:   opts.onResp,
		onFinish: opts.onFinish,

		logger: opts.logger,
	}
}

type HTTPCaller struct {
	client http.Client

	targetRPS          int
	maxConcurrentCalls int

	url     string
	headers http.Header
	method  string
	body    []byte
	timeout time.Duration

	isValidResponse func(response *http.Response, body []byte) bool // TODO: annotate

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

	onResp   func(*HTTPCallInfo)
	onFinish func(*HTTPStatistics)

	logger Logger
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
	if c.onFinish != nil {
		c.onFinish(c.Statistics())
	}
	return nil
}

// TODO: rename
func (c *HTTPCaller) runWorkCreator(ctx context.Context) {
	c.doneWg.Add(1)
	go func() {
		defer c.doneWg.Done()

		freq := int(time.Second) / c.targetRPS // TODO: explanation comment
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
	for i := 0; i < c.maxConcurrentCalls; i++ {
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
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, c.method, c.url, bytes.NewReader(c.body))
	if err != nil {
		return err // TODO: wrap
	}
	req.Header = c.headers

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
	if c.isValidResponse != nil {
		isValidResponse = c.isValidResponse(resp, body)
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

	if c.onResp != nil {
		c.onResp(&HTTPCallInfo{
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
