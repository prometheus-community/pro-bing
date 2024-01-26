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
	defaultHTTPMaxConcurrentCalls = 1
	defaultHTTPMethod             = http.MethodGet
	defaultTimeout                = time.Second * 10
)

type httpCallerOptions struct {
	client *http.Client

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

// HTTPCallerOption represents a function type for a functional parameter passed to a NewHttpCaller constructor.
type HTTPCallerOption func(options *httpCallerOptions)

// WithHTTPCallerClient is a functional parameter for a WithHTTPCallerClient which specifies a http.Client.
func WithHTTPCallerClient(client *http.Client) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.client = client
	}
}

// WithHTTPCallerTargetRPS is a functional parameter for a WithHTTPCallerClient which specifies a rps. If this option
// is not provided the default one will be used. You can check default value in const defaultHTTPTargetRPS.
func WithHTTPCallerTargetRPS(rps int) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.targetRPS = rps
	}
}

// WithHTTPCallerMaxConcurrentCalls is a functional parameter for a WithHTTPCallerClient which specifies a number of
// maximum concurrent calls. If this option is not provided the default one will be used. You can check default value in const
// defaultHTTPMaxConcurrentCalls.
func WithHTTPCallerMaxConcurrentCalls(max int) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.maxConcurrentCalls = max
	}
}

// WithHTTPCallerHeaders is a functional parameter for a WithHTTPCallerClient which specifies headers that should be
// set in request.
func WithHTTPCallerHeaders(headers http.Header) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.headers = headers
	}
}

// WithHTTPCallerMethod is a functional parameter for a WithHTTPCallerClient which specifies a method that should be
// set in request. If this option is not provided the default one will be used. You can check default value in const
// defaultHTTPMethod.
func WithHTTPCallerMethod(method string) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.method = method
	}
}

// WithHTTPCallerBody is a functional parameter for a WithHTTPCallerClient which specifies a body that should be set
// in request.
func WithHTTPCallerBody(body []byte) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.body = body
	}
}

// WithHTTPCallerTimeout is a functional parameter for a WithHTTPCallerTimeout which specifies request timeout.
// If this option is not provided the default one will be used. You can check default value in const defaultTimeout.
func WithHTTPCallerTimeout(timeout time.Duration) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.timeout = timeout
	}
}

// WithHTTPCallerIsValidResponse is a functional parameter for a WithHTTPCallerTimeout which specifies a function that
// will be used to assess whether a response is valid. If not specified, all responses will be treated as valid.
// You can read more explanation about this parameter in HTTPCaller annotation.
func WithHTTPCallerIsValidResponse(isValid func(response *http.Response, body []byte) bool) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.isValidResponse = isValid
	}
}

// WithHTTPCallerOnResp is a functional parameter for a WithHTTPCallerTimeout which specifies a callback that will be
// called when response is received. You can read more explanation about this parameter in HTTPCaller annotation.
func WithHTTPCallerOnResp(onResp func(*HTTPCallInfo)) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.onResp = onResp
	}
}

// WithHTTPCallerOnFinish is a functional parameter for a WithHTTPCallerTimeout which specifies a callback that will be
// called when probing cycle is finished. You can read more explanation about this parameter in HTTPCaller annotation.
func WithHTTPCallerOnFinish(onFinish func(statistics *HTTPStatistics)) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.onFinish = onFinish
	}
}

// WithHTTPCallerLogger is a functional parameter for a WithHTTPCallerTimeout which specifies a logger.
// If not specified, logs will be omitted.
func WithHTTPCallerLogger(logger Logger) HTTPCallerOption {
	return func(options *httpCallerOptions) {
		options.logger = logger
	}
}

// NewHttpCaller returns a new HTTPCaller. URL parameter is the only required one, other options might be specified via
// functional parameters, otherwise default values will be used where applicable.
func NewHttpCaller(url string, options ...HTTPCallerOption) *HTTPCaller {
	opts := httpCallerOptions{
		targetRPS:          defaultHTTPTargetRPS,
		maxConcurrentCalls: defaultHTTPMaxConcurrentCalls,
		method:             defaultHTTPMethod,
		timeout:            defaultTimeout,
		client:             &http.Client{},
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

// HTTPCaller represents a prober performing http calls and collecting relevant statistics.
type HTTPCaller struct {
	client *http.Client

	// targetRPS is a RPS which prober will try to maintain. You might need to increase maxConcurrentCalls value to
	// achieve required value.
	targetRPS int

	// maxConcurrentCalls is a maximum number of calls that might be performed concurrently. In other words,
	// a number of "workers" that will try to perform probing concurrently.
	// Default number is specified in defaultHTTPMaxConcurrentCalls
	maxConcurrentCalls int

	// url is a url which will be used in all probe requests, mandatory in constructor.
	url string

	// headers are headers that which will be used in all probe requests, default are none.
	headers http.Header

	// method is a http request method which will be used in all probe requests,
	// default is specified in defaultHTTPMethod
	method string

	// body is a http request body which will be used in all probe requests, default is none.
	body []byte

	// timeout is a http call timeout, default is specified in defaultTimeout.
	timeout time.Duration

	// isValidResponse is a function that will be used to validate whether a response is valid up to clients choice.
	// You can think of it as a verification that response contains data that you expected. This information will be
	// passed back in HTTPCallInfo during an onResp callback and HTTPStatistics during an onFinish callback
	// or a Statistics call.
	isValidResponse func(response *http.Response, body []byte) bool

	statsMu             sync.Mutex
	statusCodesCount    map[int]int
	validResponsesCount int
	totalLatency        statsSet
	dnsLatency          statsSet
	connLatency         statsSet
	tlsLatency          statsSet
	pureCallLatency     statsSet

	workChan chan struct{}
	doneChan chan struct{}
	doneWg   sync.WaitGroup

	// onResp is a callback, which is called when response is received
	onResp func(*HTTPCallInfo)

	// onFinish is a callback, which is called when probing session is finished
	onFinish func(*HTTPStatistics)

	// logger is a logger implementation, default is none.
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

func (s *statsSet) toPublicSet() HTTPStatisticsSet {
	return HTTPStatisticsSet{
		Min:    s.min,
		Max:    s.max,
		Avg:    s.avg,
		StdDev: s.stdDev,
	}
}

func (s *statsSet) updateByTimePair(tp timePair) {
	if !tp.isValid() {
		return
	}
	s.count++
	s.update(tp.getDuration())
}

func (s *statsSet) update(newVal time.Duration) {
	if s.min == 0 || newVal < s.min {
		s.min = newVal
	}
	if newVal > s.max {
		s.max = newVal
	}
	s.stdDev, s.stdDevM2, s.avg = calculateStdDev(s.count, newVal, s.avg, s.stdDevM2)
}

// Stop gracefully stops the execution of a HTTPCaller.
func (c *HTTPCaller) Stop() {
	close(c.doneChan)
	c.doneWg.Wait()
}

// Run starts execution of a probing.
func (c *HTTPCaller) Run() {
	c.run(context.Background())
}

// RunWithContext starts execution of a probing and allows providing a context.
func (c *HTTPCaller) RunWithContext(ctx context.Context) {
	c.run(ctx)
}

func (c *HTTPCaller) run(ctx context.Context) {
	c.runWorkScheduler(ctx)
	c.runCallers(ctx)
	c.doneWg.Wait()
	if c.onFinish != nil {
		c.onFinish(c.Statistics())
	}
}

// getCallFrequency calculates a frequency which should be used to maintain required RPS.
func (c *HTTPCaller) getCallFrequency() time.Duration {
	return time.Second / time.Duration(c.targetRPS)
}

func (c *HTTPCaller) runWorkScheduler(ctx context.Context) {
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

type timePair struct {
	start time.Time
	end   time.Time
}

func (p timePair) isValid() bool {
	return !p.start.IsZero() && !p.end.IsZero()
}

func (p timePair) getDuration() time.Duration {
	return p.end.Sub(p.start)
}

type callStatTimers struct {
	generalCall timePair
	dns         timePair
	conn        timePair
	tls         timePair
	pureCall    timePair
}

func getClientTrace(timers *callStatTimers) *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			timers.dns.start = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			timers.dns.end = time.Now()
		},
		ConnectStart: func(network, addr string) {
			timers.conn.start = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			timers.conn.end = time.Now()
		},
		TLSHandshakeStart: func() {
			timers.tls.start = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			timers.tls.end = time.Now()
		},
		WroteHeaders: func() {
			timers.pureCall.start = time.Now()
		},
		GotFirstResponseByte: func() {
			timers.pureCall.end = time.Now()
		},
	}
}

func (c *HTTPCaller) addStats(statusCode int, isValidResponse bool, timers callStatTimers) {
	// TODO: channels?
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	c.statusCodesCount[statusCode]++
	if isValidResponse {
		c.validResponsesCount++
	}
	c.totalLatency.updateByTimePair(timers.generalCall)
	c.dnsLatency.updateByTimePair(timers.dns)
	c.connLatency.updateByTimePair(timers.conn)
	c.tlsLatency.updateByTimePair(timers.tls)
	c.pureCallLatency.updateByTimePair(timers.pureCall)
}

// TODO: check http client effective lifehacks
func (c *HTTPCaller) makeCall(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	statTimers := callStatTimers{}
	trace := getClientTrace(&statTimers)

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), c.method, c.url, bytes.NewReader(c.body))
	if err != nil {
		return err // TODO: wrap
	}
	req.Header = c.headers

	statTimers.generalCall.start = time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body) // TODO: ok? think about it
	if err != nil {
		return err // TODO: wrap
	}
	resp.Body.Close() // TODO: err?
	statTimers.generalCall.end = time.Now()
	isValidResponse := true
	if c.isValidResponse != nil {
		isValidResponse = c.isValidResponse(resp, body)
	}
	c.addStats(resp.StatusCode, isValidResponse, statTimers)
	if c.onResp != nil {
		c.onResp(formHTTPCallInfo(resp.StatusCode, isValidResponse, statTimers))
	}
	return nil
}

// Statistics returns current aggregation statistics of the ongoing probing session.
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

// HTTPCallInfo represents a data set which passed as a function argument to an onResp callback.
type HTTPCallInfo struct {
	// RequestTime is a time when an overall http call was started.
	RequestTime time.Time

	// ResponseTime is a time when an overall http call was finished.
	ResponseTime time.Time

	// DNSStartTime is a time when a dns request started. Will be equal to time.Zero if dns resolving wasn't triggered.
	DNSStartTime time.Time

	// DNSDoneTime is a time when a dns response received. Will be equal to time.Zero if dns resolving wasn't triggered.
	DNSDoneTime time.Time

	// ConnStartTime is a time when a connection establishing started.
	ConnStartTime time.Time

	// ConnDoneTime is a time when a connection was successfully established.
	ConnDoneTime time.Time

	// TLSStartTime is a time when a TLS handshake started. Will be equal to time.Zero if it's not a https call.
	TLSStartTime time.Time

	// TLSEndTime is a time when a TLS handshake finished. Will be equal to time.Zero if it's not a https call.
	TLSEndTime time.Time

	// RequestHeadersWrittenTime is a time when all request headers were written. Can be used in pair
	// with ResponseFirstByteReceivedTime to calculate "pure" latency. TODO: think about a better term then "pure"
	RequestHeadersWrittenTime time.Time

	// ResponseFirstByteReceivedTime is a time when first response bytes were received. Can be used in pair with
	// RequestHeadersWrittenTime to calculate "pure" latency. TODO: think about a better term then "pure"
	ResponseFirstByteReceivedTime time.Time

	// StatusCode is a response status code
	StatusCode int

	// IsValidResponse represents a fact of whether a response is treated as valid. You can read more about it in
	// HTTPCaller annotation.
	IsValidResponse bool // TODO: think about the naming here, ANNOTATE!
}

// HTTPStatistics represents the overall probing session statistics.
type HTTPStatistics struct {
	// StatusCodesCount contains pairs of codes with counters of their appearances, where a key is a code and value is
	// a counter
	StatusCodesCount map[int]int

	// CallsCount is an overall number of performed calls
	CallsCount int

	// ValidResponsesCount is a number of responses that were treated a valid. You can read more about it in
	// HTTPCaller annotation.
	ValidResponsesCount int

	// TotalLatency contains statistical aggregations for a full call latencies.
	TotalLatency HTTPStatisticsSet

	// DNSLatency contains statistical aggregations for a dns resolvement.
	DNSLatency HTTPStatisticsSet

	// ConnLatency contains statistical aggregations for connection establishments.
	ConnLatency HTTPStatisticsSet

	// TLSLatency contains statistical aggregations for tls handshakes.
	TLSLatency HTTPStatisticsSet

	// PureCallLatency contains statistical aggregations for pure call latencies.
	PureCallLatency HTTPStatisticsSet // TODO: think about a better term then "pure"
}

func formHTTPCallInfo(statusCode int, isValidResponse bool, statTimers callStatTimers) *HTTPCallInfo {
	return &HTTPCallInfo{
		RequestTime:                   statTimers.generalCall.start,
		ResponseTime:                  statTimers.generalCall.end,
		DNSStartTime:                  statTimers.dns.start,
		DNSDoneTime:                   statTimers.dns.end,
		ConnStartTime:                 statTimers.conn.start,
		ConnDoneTime:                  statTimers.conn.end,
		TLSStartTime:                  statTimers.tls.start,
		TLSEndTime:                    statTimers.tls.end,
		RequestHeadersWrittenTime:     statTimers.pureCall.start,
		ResponseFirstByteReceivedTime: statTimers.pureCall.end,

		StatusCode:      statusCode,
		IsValidResponse: isValidResponse,
	}
}

// HTTPStatisticsSet is a common reused set of standard statistical functions.
type HTTPStatisticsSet struct {
	Min    time.Duration
	Max    time.Duration
	Avg    time.Duration
	StdDev time.Duration
}
