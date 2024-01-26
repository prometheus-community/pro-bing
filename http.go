package probing

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
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
	validResponsesCount int
	totalLatency        statsSet
	dnsLatency          statsSet
	connLatency         statsSet
	tlsLatency          statsSet
	pureCallLatency     statsSet // TODO: annotate

	workChan chan struct{}
	doneChan chan struct{}
	doneWg   sync.WaitGroup

	onResp   func(*HTTPCallInfo)
	onFinish func(*HTTPStatistics)

	logger Logger
}

type statsSet struct {
	count    int
	min      time.Duration
	max      time.Duration
	avg      time.Duration
	stdDevM2 time.Duration
	stdDev   time.Duration
}

func (s statsSet) toPublicSet() HTTPStatisticsSet {
	return HTTPStatisticsSet{
		Min:    s.min,
		Max:    s.max,
		Avg:    s.avg,
		StdDev: s.stdDev,
	}
}

func (s *statsSet) update(newVal time.Duration, count int) {
	if s.min == 0 || newVal < s.min {
		s.min = newVal
	}
	if newVal > s.max {
		s.max = newVal
	}
	s.stdDev, s.stdDevM2, s.avg = calculateStdDev(count, newVal, s.avg, s.stdDevM2)
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

// TODO: proper annotation & explanation
func (c *HTTPCaller) getCallFrequency() time.Duration {
	return time.Second / time.Duration(c.targetRPS)
}

// TODO: rename
func (c *HTTPCaller) runWorkCreator(ctx context.Context) {
	c.doneWg.Add(1)
	go func() {
		defer c.doneWg.Done()

		ticker := time.NewTicker(c.getCallFrequency())
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

	var (
		dnsStart                time.Time
		dnsDone                 time.Time
		connStart               time.Time
		connDone                time.Time
		tlsStart                time.Time
		tlsDone                 time.Time
		requestHeadersWriteDone time.Time
		responseFirstByteDone   time.Time
	)

	trace := httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			dnsStart = time.Now()
		},
		ConnectStart: func(network, addr string) {
			connStart = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			connDone = time.Now()
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			tlsDone = time.Now()
		},
		WroteHeaders: func() {
			requestHeadersWriteDone = time.Now()
		},
		GotFirstResponseByte: func() {
			responseFirstByteDone = time.Now()
		},
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, &trace), c.method, c.url, bytes.NewReader(c.body))
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
	totalLatency := responseTime.Sub(requestTime)
	isValidResponse := true
	if c.isValidResponse != nil {
		isValidResponse = c.isValidResponse(resp, body)
	}

	// TODO: channels?
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.statusCodesCount[resp.StatusCode]++
	if isValidResponse {
		c.validResponsesCount++
	}
	c.totalLatency.count++
	c.totalLatency.update(totalLatency, c.totalLatency.count)
	if !dnsStart.IsZero() && !dnsDone.IsZero() {
		c.dnsLatency.count++
		c.dnsLatency.update(dnsDone.Sub(dnsStart), c.dnsLatency.count)
	}
	if !connStart.IsZero() && !connDone.IsZero() {
		c.connLatency.count++
		c.connLatency.update(connDone.Sub(connStart), c.connLatency.count)
	}
	if !tlsStart.IsZero() && !tlsDone.IsZero() {
		c.tlsLatency.count++
		c.tlsLatency.update(tlsDone.Sub(tlsStart), c.tlsLatency.count)
	}
	c.pureCallLatency.count++ // technically obsolete, keeping for the same style
	c.pureCallLatency.update(responseFirstByteDone.Sub(requestHeadersWriteDone), c.pureCallLatency.count)

	if c.onResp != nil {
		c.onResp(&HTTPCallInfo{
			RequestTime:                   requestTime,
			ResponseTime:                  responseTime,
			DNSStartTime:                  dnsStart,
			DNSDoneTime:                   dnsDone,
			ConnStartTime:                 connStart,
			ConnDoneTime:                  connDone,
			TLSStartTime:                  tlsStart,
			RequestHeadersWrittenTime:     requestHeadersWriteDone,
			ResponseFirstByteReceivedTime: responseFirstByteDone,

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
		CallsCount:          c.totalLatency.count,
		ValidResponsesCount: c.validResponsesCount,

		TotalLatency:    c.totalLatency.toPublicSet(),
		DNSLatency:      c.dnsLatency.toPublicSet(),
		ConnLatency:     c.connLatency.toPublicSet(),
		TLSLatency:      c.tlsLatency.toPublicSet(),
		PureCallLatency: c.pureCallLatency.toPublicSet(),
	}
}

// TODO: do we want to provide a resopnse? Think about it
type HTTPCallInfo struct {
	RequestTime                   time.Time
	ResponseTime                  time.Time
	DNSStartTime                  time.Time
	DNSDoneTime                   time.Time
	ConnStartTime                 time.Time
	ConnDoneTime                  time.Time
	TLSStartTime                  time.Time
	RequestHeadersWrittenTime     time.Time
	ResponseFirstByteReceivedTime time.Time

	StatusCode      int
	IsValidResponse bool // TODO: think about the naming here, ANNOTATE!
}

// TODO: annotate all fields
type HTTPStatistics struct {
	StatusCodesCount    map[int]int
	CallsCount          int
	ValidResponsesCount int

	TotalLatency    HTTPStatisticsSet
	DNSLatency      HTTPStatisticsSet
	ConnLatency     HTTPStatisticsSet
	TLSLatency      HTTPStatisticsSet
	PureCallLatency HTTPStatisticsSet // TODO: think about name
}

type HTTPStatisticsSet struct {
	Min    time.Duration
	Max    time.Duration
	Avg    time.Duration
	StdDev time.Duration
}
