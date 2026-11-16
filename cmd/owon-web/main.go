// owon-web serves the same UI to a browser instead of a desktop window. It is
// meant for development: run it next to "npm run dev" and open the Vite page.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"reflect"
	"sync"

	"owon-hds/internal/app"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:34199", "адрес HTTP-сервера")
	dist := flag.String("dist", "frontend/dist", "папка собранного интерфейса")
	flag.Parse()

	h := &hub{clients: map[chan []byte]bool{}}
	a := app.New(h.emit)
	defer a.Shutdown()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/call", func(w http.ResponseWriter, r *http.Request) { call(w, r, a) })
	mux.HandleFunc("GET /api/events", h.serve)
	mux.Handle("/", http.FileServer(http.Dir(*dist)))
	log.Printf("http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// call invokes an App method by name, the way the desktop bindings do.
func call(w http.ResponseWriter, r *http.Request, a *app.App) {
	var req struct {
		Method string            `json:"method"`
		Args   []json.RawMessage `json:"args"`
	}
	reply := func(result any, err error) {
		resp := map[string]any{"result": result}
		if err != nil {
			resp["error"] = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reply(nil, err)
		return
	}
	m := reflect.ValueOf(a).MethodByName(req.Method)
	if !m.IsValid() || req.Method == "Shutdown" {
		http.Error(w, "unknown method", http.StatusNotFound)
		return
	}
	t := m.Type()
	if len(req.Args) != t.NumIn() {
		http.Error(w, "wrong argument count", http.StatusBadRequest)
		return
	}
	in := make([]reflect.Value, t.NumIn())
	for i := range in {
		v := reflect.New(t.In(i))
		if err := json.Unmarshal(req.Args[i], v.Interface()); err != nil {
			reply(nil, err)
			return
		}
		in[i] = v.Elem()
	}
	var result any
	var err error
	for _, out := range m.Call(in) {
		if e, ok := out.Interface().(error); ok || out.Type() == reflect.TypeFor[error]() {
			err = e
		} else {
			result = out.Interface()
		}
	}
	reply(result, err)
}

// hub fans events out to the browser over server-sent events.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
}

func (h *hub) emit(event string, data any) {
	b, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- b:
		default: // a slow page skips frames instead of stalling the loop
		}
	}
}

func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	c := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()
	fl := w.(http.Flusher)
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-c:
			w.Write([]byte("data: "))
			w.Write(b)
			w.Write([]byte("\n\n"))
			fl.Flush()
		}
	}
}
