package render

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kuba--/yag/pkg/config"
	"github.com/kuba--/yag/pkg/funcexp"
	"github.com/kuba--/yag/pkg/metrics"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	log.Println(r.RequestURI)

	// ResponseWriter wrapper
	w.Header().Set("Server", "YAG")
	w.Header().Set("Content-Type", "application/json")
	rw := &RenderResponseWriter{w: w}

	// Handler composition
	http.TimeoutHandler(&RenderHandler{}, time.Duration(config.Cfg.Webserver.Timeout)*time.Second,
		http.StatusText(http.StatusRequestTimeout)).ServeHTTP(rw, r)

	log.Printf("[%v] in %v\n", rw.Code, time.Now().Sub(t))
}

// RenderResponseWriter retrieves StatusCode from ResponseWriter
type RenderResponseWriter struct {
	Code int // the HTTP response code from WriteHeader
	w    http.ResponseWriter
}

// Header returns the response headers.
func (w *RenderResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *RenderResponseWriter) Write(buf []byte) (int, error) {
	return w.w.Write(buf)
}

// WriteHeader sets Code.
func (w *RenderResponseWriter) WriteHeader(code int) {
	w.Code = code
	w.w.WriteHeader(code)
}

// Render Handler
type RenderHandler struct {
	jsonp         string
	target        string
	from          int64
	to            int64
	maxDataPoints int
}

// GET: /render?target=my.key&from=-1.5h[&to=...&jsonp=...]
func (h *RenderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		err := h.parseQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.jsonResponse(w, funcexp.Eval(h.target, h.from, h.to, metrics.NewApi(h.maxDataPoints)))

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

/*
 * JSON Response:
 * [
 *  {"target": "status.200", "datapoints": [[1720.0, 1370846820], ...], },
 *  {"target": "status.204", "datapoints": [[1.0, 1370846820], ..., ]}
 * ]
 */
func (h *RenderHandler) jsonResponse(w http.ResponseWriter, data interface{}) {
	if m, ok := data.([]*metrics.Metrics); ok {
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "%s([", h.jsonp)
		var pos int = 0
		for _, mi := range m {
			n := len(mi.Datapoints)
			if n < 1 {
				continue
			}

			if pos > 0 {
				fmt.Fprintf(w, ",")
			}

			fmt.Fprintf(w, `{"target":"%s","datapoints":[`, mi.Target)

			startIdx := 1
			if len(mi.Datapoints[0]) > 1 {
				fmt.Fprintf(w, "[%.2f,%.0f]", mi.Datapoints[0][0], mi.Datapoints[0][1])
			} else {
				fmt.Fprintf(w, "[%.2f,%.0f]", mi.Datapoints[1][0], mi.Datapoints[1][1])
				startIdx = 2
			}

			for i := startIdx; i < n; i++ {
				fmt.Fprintf(w, ",[%.2f, %.0f]", mi.Datapoints[i][0], mi.Datapoints[i][1])
			}
			fmt.Fprintf(w, "]}")

			pos++
		}
		fmt.Fprintf(w, "])")
	} else {
		http.Error(w, fmt.Sprintf("%v", data), http.StatusBadRequest)
	}
}

// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
func (h *RenderHandler) parseQuery(r *http.Request) error {
	parseDuration := func(duration string) (time.Duration, error) {
		return time.ParseDuration(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(duration, "seconds", "s", -1), "sec", "s", -1), "minutes", "m", -1), "min", "m", -1), "hours", "h", -1))
	}

	f, err := parseDuration(r.FormValue("from"))
	if err != nil {
		return err
	}
	h.from = time.Now().Add(f).Unix()

	t := r.FormValue("to")
	if len(t) < 1 {
		h.to = time.Now().Unix()
	} else {
		t, err := parseDuration(t)
		if err != nil {
			return err
		}
		h.to = time.Now().Add(t).Unix()
	}

	if h.maxDataPoints, err = strconv.Atoi(r.FormValue("maxDataPoints")); err != nil {
		h.maxDataPoints = -1
	}

	h.jsonp = r.FormValue("jsonp")
	h.target = fmt.Sprintf("_(%s)", strings.Join(r.URL.Query()["target"], ","))

	return nil
}
