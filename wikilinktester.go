package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/PuerkitoBio/fetchbot"
	"github.com/PuerkitoBio/goquery"
)

var (
	// Protect access to dup
	mu sync.Mutex
	// Duplicates table
	dup = map[string]bool{}
)

func main() {
	lf, err := os.OpenFile("testlogfile", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer lf.Close()

	log.SetOutput(lf)
	log.Println("This is a test log entry")
	seed := "http://github.com/golang/go/wiki"
	// Parse the provided seed
	u, err := url.Parse(seed)
	fmt.Printf("Scanning: %v\n", u.String())
	if err != nil {
		log.Fatal(err)
	}
	// Create the muxer
	mux := fetchbot.NewMux()

	// Handle all errors the same
	mux.HandleErrors(fetchbot.HandlerFunc(func(ctx *fetchbot.Context, res *http.Response, err error) {
		fmt.Printf("[ERR - HandleErrors] %s\n", err)
	}))

	// Handle GET requests for html responses, to parse the body and enqueue all links as HEAD
	// requests.
	mux.Response().Method("GET").Host(u.Host).Path("/golang/go/wiki").ContentType("text/html").Handler(fetchbot.HandlerFunc(
		func(ctx *fetchbot.Context, res *http.Response, err error) {
			log.Printf("GET: %v - %v\n", res.Status, ctx.Cmd.URL())
			doc, err := goquery.NewDocumentFromResponse(res)
			if err != nil {
				fmt.Printf("[GET] %s %s - %s\n", res.Status, ctx.Cmd.URL(), err)
				return
			}
			// Enqueue all links as HEAD requests
			enqueueLinks(ctx, doc)
		}))

	// Handle GET requests for html responses coming from the source host - we don't want
	// to crawl links from other hosts.
	mux.Response().ContentType("text/html").Handler(fetchbot.HandlerFunc(
		func(ctx *fetchbot.Context, res *http.Response, err error) {
			log.Printf("HEAD: %v -  %v\n", res.Status, ctx.Cmd.URL())
			if strings.HasPrefix(res.Status, "40") || strings.HasPrefix(res.Status, "50") {
				fmt.Printf("[ERR] - %v - %v\n", res.Status, ctx.Cmd.URL())
			}
		}))
	h := logHandler(mux)
	f := fetchbot.New(h)
	f.UserAgent = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/45.0.2454.85 Safari/537.36"
	f.DisablePoliteness = true

	queue := f.Start()
	queue.SendStringHead()

	dup[seed] = true
	_, err = queue.SendStringGet(seed)
	if err != nil {
		fmt.Printf("[ERR] GET %s - %s\n", seed, err)
	}
	queue.Block()

}

// logHandler prints the fetch information and dispatches the call to the wrapped Handler.
func logHandler(wrapped fetchbot.Handler) fetchbot.Handler {
	return fetchbot.HandlerFunc(func(ctx *fetchbot.Context, res *http.Response, err error) {
		if err == nil {
			//fmt.Printf("[%d] %s %s - %s\n", res.StatusCode, ctx.Cmd.Method(), ctx.Cmd.URL(), res.Header.Get("Content-Type"))
		}
		wrapped.Handle(ctx, res, err)
	})
}

func enqueueLinks(ctx *fetchbot.Context, doc *goquery.Document) {
	mu.Lock()
	defer mu.Unlock()
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		val, _ := s.Attr("href")
		// Resolve address
		u, err := ctx.Cmd.URL().Parse(val)
		if err != nil {
			fmt.Printf("error: resolve URL %s - %s\n", val, err)
			return
		}
		hostpathOnly := strings.Replace(u.String(), "#"+u.Fragment, "", 1)

		if !dup[hostpathOnly] {
			dup[hostpathOnly] = true

			if strings.Contains(u.String(), "_history") {
				// skipping _history URL
			} else if u.Host == "github.com" && strings.Contains(u.Path, "/golang/go/wiki") {
				//fmt.Printf("  --> Sending GET for %v\n", hostpathOnly)
				if _, err := ctx.Q.SendStringGet(hostpathOnly); err != nil {
					fmt.Printf("error: enqueue get %s - %s\n", u, err)
				}
			} else if u.Scheme == "http" || u.Scheme == "https" {
				//fmt.Printf("  --> Sending HEAD for %v\n", u.String())
				if _, err := ctx.Q.SendStringHead(u.String()); err != nil {
					fmt.Printf("error: enqueue get %s - %s\n", u, err)
				}

			}

		}
	})
}
