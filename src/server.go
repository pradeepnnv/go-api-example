// Simple Kubernetes testing app
// with health and redines checks,
// graceful shutdown and Prometheus metrics
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	log "github.com/rs/zerolog/log"

	guuid "github.com/google/uuid"
)

var counter int
var port = 8081
var mutex = &sync.Mutex{}

const tout int = 10

func setup() {

	zerolog.TimeFieldFormat = ""

	zerolog.TimestampFunc = func() time.Time {
		return time.Date(2008, 1, 8, 17, 5, 05, 0, time.UTC)
	}
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

// Prometheus Counter var
var (
	curlErrorCollector = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "error_curl_total",
			Help: "Total curl request failed",
		},
		[]string{"vendor"},
	)
)

// PROMETHEUS CUSTOM METRIC
// Annotate the K8S Pods so Prometheus starts scraping
//  annotations:
//    prometheus.io/scrape: "true"
//    prometheus.io/port: "8081"
//    prometheus.io/path: "/metrics" # this is the default
func init() {
	prometheus.MustRegister(curlErrorCollector)
}

// checkRest: Rest api handler
func checkRest(w http.ResponseWriter, r *http.Request) {
	vendor := r.FormValue("vendor")

	// Simulate random failure
	err := rand.Intn(2) == 0
	if err {
		// if error increment total error
		go RecordCurlError(vendor)
		w.Write([]byte("Failed to fetch"))
	} else {
		w.Write([]byte("Vendor status: ok"))
	}
}

// RecordCurlError Error counter increment
func RecordCurlError(vendor string) {
	curlErrorCollector.With(prometheus.Labels{"vendor": vendor}).Inc()
}

// HANDLERS //
// echoString: Echo handler
func echoString(w http.ResponseWriter, r *http.Request) {
	name, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	if r.URL.Path != "/" {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "I am: "+name)

	mutex.Lock()
	counter++
	fmt.Fprintln(w, "Requests: "+strconv.Itoa(counter))
	mutex.Unlock()

	tm := time.Now().Format(time.RFC1123)
	w.Write([]byte("Time: " + tm + "\n"))

	//fmt.Fprintln(w, "Path: /", r.URL.Path[1:])
	//http.ServeFile(w, r, r.URL.Path[1:])
}

// healthz: Health handler
func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

// uuid: UUID Generator
func uuid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json;")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := guuid.New()
	log.Info().Msgf("github.com/google/uuid:         %s\n", id.String())
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, id)
}

// type testStruct struct {
// 	Test string
// }

// Prints JSON Request data
func printJSONReq(w http.ResponseWriter, r *http.Request) {
	// decoder := json.NewDecoder(r.Body)
	// var t testStruct
	// err := decoder.Decode(&t)
	// if err != nil {
	// 	panic(err)
	// }
	// log.Println(t)
	// log.Println(decoder)
	// Save a copy of this request for debugging.
	requestDump, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Println(err)
	}
	log.Info().Msg(string(requestDump))
}

// readyz: Rediness handler
func readyz(w http.ResponseWriter, r *http.Request, isReady *atomic.Value) {
	if isReady == nil || !isReady.Load().(bool) {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

// SetupCloseHandler Interupt handler
func SetupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info().Msg("\rCaught sig interrupt...exiting.")
		// Do something on exit, DeleteFiles() etc.
		os.Exit(0)
	}()
}

func main() {
	setup()
	SetupCloseHandler()

	http.Handle("/metrics", promhttp.Handler())
	// hit this api several time with query string vendor=something
	http.HandleFunc("/checkrest", checkRest)

	http.HandleFunc("/", echoString)

	// Liveness probe
	http.HandleFunc("/healthz", healthz)

	// UUID
	http.HandleFunc("/uuid", uuid)

	// UUID
	http.HandleFunc("/printJSONReq", printJSONReq)

	// Rediness probe (simulate X seconds load time)
	isReady := &atomic.Value{}
	isReady.Store(false)
	go func() {
		log.Printf("Ready NOK")
		time.Sleep(time.Duration(tout) * time.Second)
		isReady.Store(true)
		log.Printf("Ready OK")
	}()
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		readyz(w, r, isReady)
	})

	// Change the port via env var
	penv := os.Getenv("PORT")
	if penv != "" {
		eport, error := strconv.Atoi(penv)
		if error != nil {
			panic(error)
		}
		port = eport
	}
	// Change the port via command line flag
	pflag := flag.String("port", "", "service port")
	flag.Parse()
	if *pflag != "" {
		cport, error := strconv.Atoi(*pflag)
		if error != nil {
			panic(error)
		}
		port = cport
	}
	sport := ":" + strconv.Itoa(port)

	// Create server instance
	s := &http.Server{
		Addr:           sport,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    15 * time.Second,
		MaxHeaderBytes: 1 << 20, // Max header of 1MB
	}
	log.Info().Msg("Starting the service listening on port " + sport + " ...")
	//log.Fatal(http.ListenAndServe(sport, nil))
	// log.Fatal(s.ListenAndServe())
	log.Fatal().Err(s.ListenAndServe())
}
