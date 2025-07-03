package ws

import (
    "log"
    "net/http"
    "sync"
)

type LoggingRoundTripper struct {
    rt http.RoundTripper
}

func (l *LoggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
    log.Printf("HTTP Request: %s %s", req.Method, req.URL)
    resp, err := l.rt.RoundTrip(req)
    if err != nil {
        log.Printf("HTTP Request failed: %v", err)
        return nil, err
    }
    log.Printf("HTTP Response: %s %s", resp.Status, req.URL)
    return resp, nil
}

var (
    clientInstance *http.Client
    once           sync.Once
)

func GetLoggingClient() *http.Client {
    once.Do(func() {
        clientInstance = &http.Client{
            Transport: &LoggingRoundTripper{rt: http.DefaultTransport},
        }
    })
    return clientInstance
}
