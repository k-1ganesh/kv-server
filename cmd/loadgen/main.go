package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"kv-server/internal/config"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Request struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Stats struct {
	successCount   uint64
	failCount      uint64
	totalLatencyMs uint64
}

type LoadGenerator struct {
	serverURL string
	workload  string
	client    *http.Client
	stats     *Stats
}

func main() {
	// Load environment variables from .env file
	if err := config.LoadEnv(".env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Command-line flags with env variable defaults
	serverURL := flag.String("server", config.GetEnv("LOAD_SERVER_URL", "http://localhost:8080"), "Server URL")
	clients := flag.Int("clients", getEnvAsInt("LOAD_CLIENTS", 10), "Number of concurrent clients")
	duration := flag.Int("duration", getEnvAsInt("LOAD_DURATION", 60), "Test duration in seconds")
	workload := flag.String("workload", config.GetEnv("LOAD_WORKLOAD", "getput"), "Workload type: putall, getall, getpopular, getput")
	flag.Parse()

	log.Printf("Starting load generator: %d clients, %d seconds, workload=%s", *clients, *duration, *workload)

	stats := &Stats{}
	lg := &LoadGenerator{
		serverURL: *serverURL,
		workload:  *workload,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        1000,
				MaxIdleConnsPerHost: 1000,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		stats: stats,
	}

	// Warmup - populate some data
	log.Println("Warming up...")
	lg.warmup()

	// Run load test
	log.Println("Starting load test...")
	startTime := time.Now()

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	for i := 0; i < *clients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			lg.runClient(clientID, stopChan)
		}(i)
	}

	// Stop after duration
	time.Sleep(time.Duration(*duration) * time.Second)
	close(stopChan)
	wg.Wait()

	elapsed := time.Since(startTime).Seconds()

	// Print results
	lg.printResults(elapsed)
}

func (lg *LoadGenerator) warmup() {
	// Populate 100000 keys for testing
	for i := 0; i < 100000; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := fmt.Sprintf("value_%d", i)
		lg.createKey(key, value)
	}
}

func (lg *LoadGenerator) runClient(clientID int, stopChan chan struct{}) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)))

	for {
		select {
		case <-stopChan:
			return
		default:
			lg.executeRequest(rng)
		}
	}
}

func (lg *LoadGenerator) executeRequest(rng *rand.Rand) {
	start := time.Now()
	var err error

	switch lg.workload {
	case "putall":
		err = lg.workloadPutAll(rng)
	case "getall":
		err = lg.workloadGetAll(rng)
	case "getpopular":
		err = lg.workloadGetPopular(rng)
	case "getput":
		err = lg.workloadGetPut(rng)
	default:
		err = lg.workloadGetPut(rng)
	}

	latency := time.Since(start).Microseconds()
	atomic.AddUint64(&lg.stats.totalLatencyMs, uint64(latency))

	if err != nil {
		atomic.AddUint64(&lg.stats.failCount, 1)
	} else {
		atomic.AddUint64(&lg.stats.successCount, 1)
	}
}

func (lg *LoadGenerator) workloadPutAll(rng *rand.Rand) error {
	if rng.Intn(2) == 0 {
		// Create
		key := fmt.Sprintf("key_%d", rng.Intn(10000))
		value := fmt.Sprintf("value_%d", rng.Intn(10000))
		return lg.createKey(key, value)
	}
	// Delete
	key := fmt.Sprintf("key_%d", rng.Intn(10000))
	return lg.deleteKey(key)
}

func (lg *LoadGenerator) workloadGetAll(rng *rand.Rand) error {
	// Read with unique keys (cache miss)
	key := fmt.Sprintf("key_%d", rng.Intn(100000))
	return lg.readKey(key)
}

func (lg *LoadGenerator) workloadGetPopular(rng *rand.Rand) error {
	// Read from small set of popular keys (cache hit)
	key := fmt.Sprintf("key_%d", rng.Intn(1000))
	return lg.readKey(key)
}

func (lg *LoadGenerator) workloadGetPut(rng *rand.Rand) error {
	op := rng.Intn(10)
	key := fmt.Sprintf("key_%d", rng.Intn(1000))

	if op < 7 {
		// 70% reads
		return lg.readKey(key)
	} else if op < 9 {
		// 20% creates
		value := fmt.Sprintf("value_%d", rng.Intn(10000))
		return lg.createKey(key, value)
	}
	// 10% deletes
	return lg.deleteKey(key)
}

func (lg *LoadGenerator) createKey(key, value string) error {
	reqBody := Request{Key: key, Value: value}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := lg.client.Post(lg.serverURL+"/kv", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create failed: %d", resp.StatusCode)
	}
	return nil
}

func (lg *LoadGenerator) readKey(key string) error {
	resp, err := lg.client.Get(lg.serverURL + "/kv/" + key)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("read failed: %d", resp.StatusCode)
	}
	return nil
}

func (lg *LoadGenerator) deleteKey(key string) error {
	req, _ := http.NewRequest(http.MethodDelete, lg.serverURL+"/kv/"+key, nil)
	resp, err := lg.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete failed: %d", resp.StatusCode)
	}
	return nil
}

func (lg *LoadGenerator) printResults(elapsed float64) {
	success := atomic.LoadUint64(&lg.stats.successCount)
	failed := atomic.LoadUint64(&lg.stats.failCount)
	totalLatency := atomic.LoadUint64(&lg.stats.totalLatencyMs)

	total := success + failed
	throughput := float64(success) / elapsed
	avgLatency := float64(0)
	if success > 0 {
		avgLatency = float64(totalLatency) / float64(success)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("LOAD TEST RESULTS")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Duration:              %.2f seconds\n", elapsed)
	fmt.Printf("Total Requests:        %d\n", total)
	fmt.Printf("Successful Requests:   %d\n", success)
	fmt.Printf("Failed Requests:       %d\n", failed)
	fmt.Printf("Average Throughput:    %.2f requests/sec\n", throughput)
	fmt.Printf("Average Response Time: %.2f microsec\n", avgLatency)
	fmt.Println(strings.Repeat("=", 60))
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
