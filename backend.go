package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

	"strconv"

	"encoding/json"

	"github.com/gorilla/mux"
)

var (
	cache     = make(map[string]cachedPage)
	cacheLock sync.Mutex
)

type cachedPage struct {
	Content      string
	LastCrawled  time.Time
	PayingAccess bool
}

// Number of threads for paying and non-paying customers
var payingThreads int = 5
var nonPayingThreads int = 2

func setValues(w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()

	newPayingThreadsStr := queryParams.Get("paying")
	newNonPayingThreadsStr := queryParams.Get("non-paying")

	newPayingThreads, err := strconv.Atoi(newPayingThreadsStr)
	if err != nil || newPayingThreadsStr == "" {
		// http.Error(w, "param1 must be an integer", http.StatusBadRequest)
		// return
		newPayingThreads = 0
	}
	newNonPayingThreads, err := strconv.Atoi(newNonPayingThreadsStr)
	if err != nil || newNonPayingThreadsStr == "" {
		// http.Error(w, "param2 must be an integer", http.StatusBadRequest)
		// return
		newNonPayingThreads = 0
	}

	if newPayingThreads > 0 {payingThreads = newPayingThreads}
	if newNonPayingThreads > 0 {nonPayingThreads = newNonPayingThreads}
}

func getValues(w http.ResponseWriter, r *http.Request) {
	// Create a map to hold the variable values
	variables := map[string]int{"paying": payingThreads, "non-paying": nonPayingThreads}

	// Convert the map to JSON
	response, err := json.Marshal(variables)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond with the JSON containing variable values
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func crawlPage(URL string, payingCustomer bool, wg *sync.WaitGroup) {
	defer wg.Done()

	cacheLock.Lock()
	defer cacheLock.Unlock()

	if page, exists := cache[URL]; exists && time.Since(page.LastCrawled).Minutes() < 60 && (payingCustomer || page.PayingAccess) {
		return
	}

	content, err := fetchPage(URL)
	if err != nil {
		fmt.Printf("Error crawling page %s: %v\n", URL, err)
		return
	}

	cache[URL] = cachedPage{
		Content:      content,
		LastCrawled:  time.Now(),
		PayingAccess: payingCustomer,
	}
}

func fetchPage(URL string) (string, error) {
	resp, err := http.Get(URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()
	URL := queryParams.Get("url")
	payingCustomer := queryParams.Get("paying_customer") == "true"

	if URL == "" {
		http.Error(w, "URL parameter is required", http.StatusBadRequest)
		return
	}

	var wg sync.WaitGroup

	// // Number of threads for paying and non-paying customers
	// payingThreads := 5
	// nonPayingThreads := 2

	// Add goroutines to the wait group
	for i := 0; i < payingThreads; i++ {
		wg.Add(1)
		go crawlPage(URL, payingCustomer, &wg)
	}

	for i := 0; i < nonPayingThreads; i++ {
		wg.Add(1)
		go crawlPage(URL, false, &wg)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	content, exists := cache[URL]
	if !exists {
		http.Error(w, "Error crawling page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(content.Content))
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET")
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/crawl", handleRequest).Methods("GET")

	r.HandleFunc("/set", setValues).Methods("POST")
	r.HandleFunc("/get", getValues).Methods("GET")

	// Middleware to enable CORS
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enableCors(&w)
			next.ServeHTTP(w, r)
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	fmt.Printf("Server is running on port %s...\n", port)
	http.ListenAndServe(":"+port, r)
}
