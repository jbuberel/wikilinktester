package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	seed := "http://github.com/golang/go/wiki"
	// Parse the provided seed
	u, err := url.Parse(seed)
	if err != nil {
		log.Fatal(err)
	}
	// Create the muxer
	mux := fetchbot.NewMux()

	// Handle all errors the same
	mux.HandleErrors(fetchbot.HandlerFunc(func(ctx *fetchbot.Context, res *http.Response, err error) {
		fmt.Printf("[ERR] %s %s - %s\n", ctx.Cmd.Method(), ctx.Cmd.URL(), err)
	}))

	// Handle GET requests for html responses, to parse the body and enqueue all links as HEAD
	// requests.
	mux.Response().Method("GET").ContentType("text/html").Handler(fetchbot.HandlerFunc(
		func(ctx *fetchbot.Context, res *http.Response, err error) {
			// Process the body to find the links
			doc, err := goquery.NewDocumentFromResponse(res)
			if err != nil {
				fmt.Printf("[ERR] %s %s - %s\n", ctx.Cmd.Method(), ctx.Cmd.URL(), err)
				return
			}
			// Enqueue all links as HEAD requests
			enqueueLinks(ctx, doc)
		}))

	// Handle HEAD requests for html responses coming from the source host - we don't want
	// to crawl links from other hosts.
	mux.Response().Method("HEAD").Host(u.Host).ContentType("text/html").Handler(fetchbot.HandlerFunc(
		func(ctx *fetchbot.Context, res *http.Response, err error) {
			if _, err := ctx.Q.SendStringGet(ctx.Cmd.URL().String()); err != nil {
				fmt.Printf("[ERR] %s %s - %s\n", ctx.Cmd.Method(), ctx.Cmd.URL(), err)
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
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		val, _ := s.Attr("href")
		// Resolve address
		u, err := ctx.Cmd.URL().Parse(val)
		if err != nil {
			fmt.Printf("error: resolve URL %s - %s\n", val, err)
			return
		}
    hostpathOnly := strings.Replace(u.String(),"#" + u.Fragment, "", 1)

		if !dup[hostpathOnly] {
			if u.Host == "github.com" && strings.Contains(u.Path, "/golang/go/wiki") {
				fmt.Printf("  --> Sending GET for %v\n", hostpathOnly)
				if _, err := ctx.Q.SendStringGet(hostpathOnly); err != nil {
					fmt.Printf("error: enqueue head %s - %s\n", u, err)
				} else {
					dup[u.String()] = true
				}
			} else {
				fmt.Printf("  --> Sending HEAD for %v\n", u.String())
				if _, err := ctx.Q.SendStringHead(u.String()); err != nil {
					fmt.Printf("error: enqueue head %s - %s\n", u, err)
				} else {
					dup[u.String()] = true
				}

			}

		}
	})
	mu.Unlock()
}
