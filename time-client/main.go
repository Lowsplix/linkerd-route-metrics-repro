package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/kostyay/grpc-web-example/time/goclient/api/time/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var (
	serverAddress = flag.String("server", "localhost:9090", "The server address to connect to (host:port)")
	requestRate   = flag.Duration("rate", 5*time.Second, "Time between requests (e.g., 1s, 500ms)")
	maxRequests   = flag.Int("max-requests", 0, "Maximum number of requests to send (0 for unlimited)")
	timeout       = flag.Duration("timeout", 10*time.Second, "Request timeout")
	httpPort      = flag.String("http-port", ":8080", "HTTP server port for health checks and metrics")
)

type TimeClient struct {
	client     pb.TimeServiceClient
	conn       *grpc.ClientConn
	ctx        context.Context
	cancel     context.CancelFunc
	requestNum int
	stats      *ClientStats
	httpServer *http.Server
}

type ClientStats struct {
	TotalRequests      int
	SuccessfulRequests int
	FailedRequests     int
	TotalLatency       time.Duration
	MinLatency         time.Duration
	MaxLatency         time.Duration
	StartTime          time.Time
}

func initLogger() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
}

func NewTimeClient(serverAddr string) (*TimeClient, error) {
	slog.Info("Connecting to time service", "address", serverAddr)

	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := pb.NewTimeServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())

	stats := &ClientStats{
		MinLatency: time.Duration(^uint64(0) >> 1), // Max duration
		MaxLatency: 0,
		StartTime:  time.Now(),
	}

	tc := &TimeClient{
		client: client,
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
		stats:  stats,
	}

	// Setup HTTP server
	tc.setupHTTPServer()

	return tc, nil
}

func (tc *TimeClient) setupHTTPServer() {
	mux := http.NewServeMux()
	
	// Health check endpoint
	mux.HandleFunc("/health", tc.healthHandler)
	mux.HandleFunc("/healthz", tc.healthHandler) // Alternative health endpoint
	
	// Readiness check endpoint
	mux.HandleFunc("/ready", tc.readinessHandler)
	
	// Metrics endpoint
	mux.HandleFunc("/metrics", tc.metricsHandler)
	
	// Status endpoint with detailed info
	mux.HandleFunc("/status", tc.statusHandler)

	tc.httpServer = &http.Server{
		Addr:    *httpPort,
		Handler: mux,
	}
}

func (tc *TimeClient) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (tc *TimeClient) readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Check if we can connect to the gRPC server
	if tc.conn == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Not Ready - No gRPC connection"))
		return
	}
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

func (tc *TimeClient) metricsHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(tc.stats.StartTime)
	var avgLatency time.Duration
	if tc.stats.TotalRequests > 0 {
		avgLatency = tc.stats.TotalLatency / time.Duration(tc.stats.TotalRequests)
	}

	// Simple metrics format (could be Prometheus format if needed)
	metrics := fmt.Sprintf(`# HELP time_client_requests_total Total number of requests
# TYPE time_client_requests_total counter
time_client_requests_total %d

# HELP time_client_requests_successful_total Total number of successful requests
# TYPE time_client_requests_successful_total counter
time_client_requests_successful_total %d

# HELP time_client_requests_failed_total Total number of failed requests
# TYPE time_client_requests_failed_total counter
time_client_requests_failed_total %d

# HELP time_client_uptime_seconds Uptime in seconds
# TYPE time_client_uptime_seconds gauge
time_client_uptime_seconds %f

# HELP time_client_avg_latency_seconds Average request latency in seconds
# TYPE time_client_avg_latency_seconds gauge
time_client_avg_latency_seconds %f
`,
		tc.stats.TotalRequests,
		tc.stats.SuccessfulRequests,
		tc.stats.FailedRequests,
		uptime.Seconds(),
		avgLatency.Seconds(),
	)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(metrics))
}

func (tc *TimeClient) statusHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(tc.stats.StartTime)
	var avgLatency time.Duration
	var successRate float64

	if tc.stats.TotalRequests > 0 {
		avgLatency = tc.stats.TotalLatency / time.Duration(tc.stats.TotalRequests)
		successRate = float64(tc.stats.SuccessfulRequests) / float64(tc.stats.TotalRequests) * 100
	}

	status := map[string]interface{}{
		"status":             "running",
		"uptime":             uptime.String(),
		"total_requests":     tc.stats.TotalRequests,
		"successful_requests": tc.stats.SuccessfulRequests,
		"failed_requests":    tc.stats.FailedRequests,
		"success_rate":       fmt.Sprintf("%.2f%%", successRate),
		"avg_latency":        avgLatency.String(),
		"min_latency":        tc.stats.MinLatency.String(),
		"max_latency":        tc.stats.MaxLatency.String(),
		"server_address":     *serverAddress,
		"request_rate":       requestRate.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (tc *TimeClient) startHTTPServer() {
	go func() {
		slog.Info("Starting HTTP server", "port", *httpPort)
		if err := tc.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
		}
	}()
}

func (tc *TimeClient) Close() {
	tc.cancel()
	
	// Shutdown HTTP server
	if tc.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tc.httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown failed", "error", err)
		}
	}
	
	if tc.conn != nil {
		tc.conn.Close()
	}
}

func (tc *TimeClient) SendRequest() error {
	tc.requestNum++
	tc.stats.TotalRequests++

	start := time.Now()

	ctx, cancel := context.WithTimeout(tc.ctx, *timeout)
	defer cancel()

	req := &pb.GetCurrentTimeRequest{}

	slog.Info("Sending request", "request_num", tc.requestNum)

	resp, err := tc.client.GetCurrentTime(ctx, req)
	latency := time.Since(start)

	tc.stats.TotalLatency += latency
	if latency < tc.stats.MinLatency {
		tc.stats.MinLatency = latency
	}
	if latency > tc.stats.MaxLatency {
		tc.stats.MaxLatency = latency
	}

	if err != nil {
		tc.stats.FailedRequests++
		if st, ok := status.FromError(err); ok {
			slog.Error("Request failed",
				"request_num", tc.requestNum,
				"error", st.Message(),
				"code", st.Code(),
				"latency", latency)
		} else {
			slog.Error("Request failed",
				"request_num", tc.requestNum,
				"error", err,
				"latency", latency)
		}
		return err
	}

	tc.stats.SuccessfulRequests++
	slog.Info("Request successful",
		"request_num", tc.requestNum,
		"response_time", resp.CurrentTime,
		"latency", latency)

	return nil
}

func (tc *TimeClient) PrintStats() {
	if tc.stats.TotalRequests == 0 {
		slog.Info("No requests sent yet")
		return
	}

	avgLatency := tc.stats.TotalLatency / time.Duration(tc.stats.TotalRequests)
	successRate := float64(tc.stats.SuccessfulRequests) / float64(tc.stats.TotalRequests) * 100

	slog.Info("Client Statistics",
		"total_requests", tc.stats.TotalRequests,
		"successful_requests", tc.stats.SuccessfulRequests,
		"failed_requests", tc.stats.FailedRequests,
		"success_rate", fmt.Sprintf("%.2f%%", successRate),
		"avg_latency", avgLatency,
		"min_latency", tc.stats.MinLatency,
		"max_latency", tc.stats.MaxLatency)
}

func (tc *TimeClient) Run() {
	// Start HTTP server
	tc.startHTTPServer()

	ticker := time.NewTicker(*requestRate)
	defer ticker.Stop()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("Starting time client",
		"server", *serverAddress,
		"request_rate", *requestRate,
		"max_requests", *maxRequests,
		"timeout", *timeout,
		"http_port", *httpPort)

	// Send initial request immediately
	tc.SendRequest()

	for {
		select {
		case <-tc.ctx.Done():
			slog.Info("Context cancelled, shutting down")
			return
		case <-sigChan:
			slog.Info("Received shutdown signal")
			tc.PrintStats()
			return
		case <-ticker.C:
			if *maxRequests > 0 && tc.requestNum >= *maxRequests {
				slog.Info("Reached maximum number of requests", "max_requests", *maxRequests)
				tc.PrintStats()
				return
			}
			tc.SendRequest()
		}
	}
}

func main() {
	flag.Parse()
	initLogger()

	client, err := NewTimeClient(*serverAddress)
	if err != nil {
		slog.Error("Failed to create client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	// Print stats every 30 seconds
	go func() {
		statsTicker := time.NewTicker(30 * time.Second)
		defer statsTicker.Stop()

		for {
			select {
			case <-client.ctx.Done():
				return
			case <-statsTicker.C:
				client.PrintStats()
			}
		}
	}()

	client.Run()

	slog.Info("Time client shutting down")
	client.PrintStats()
}
