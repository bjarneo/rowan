//go:build devserve

package main

// The dev server serves the UI and the bound App methods over HTTP, so the
// frontend can run in a headless browser for tests. It is not part of
// release builds.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
)

const shim = `<script>
(() => {
  const handlers = {}
  window.go = { main: { App: new Proxy({}, { get: (_, m) => (...args) =>
    fetch('/rpc/' + m, { method: 'POST', body: JSON.stringify(args) }).then((r) => r.json()).then((x) => {
      if (x.error) throw x.error
      return x.result
    }) }) } }
  window.runtime = {
    EventsOnMultiple(name, cb) { (handlers[name] ||= []).push(cb); return () => { handlers[name] = handlers[name].filter((x) => x !== cb) } },
    EventsOff() {}, EventsEmit() {},
    ClipboardSetText: async (t) => { window.__clip = t; return true },
    LogPrint() {}, LogTrace() {}, LogDebug() {}, LogInfo() {}, LogWarning() {}, LogError() {}, LogFatal() {},
  }
  const es = new EventSource('/events')
  es.onmessage = (e) => { const m = JSON.parse(e.data); (handlers[m.name] || []).forEach((cb) => cb(m.data)) }
})()
</script>`

func init() { devServe = serveDev }

func serveDev(app *App, addr string) error {
	var mu sync.Mutex
	subs := map[chan []byte]bool{}
	app.emitHook = func(name string, data any) {
		b, _ := json.Marshal(map[string]any{"name": name, "data": data})
		mu.Lock()
		for ch := range subs {
			select {
			case ch <- b:
			default:
			}
		}
		mu.Unlock()
	}
	// Wails calls OnStartup in a goroutine while the webview loads. Do the same
	// here, so this harness sees the same order of events as the real window.
	go app.startup(context.Background())
	var dist fs.FS
	if d := os.Getenv("ROWAN_DIST"); d != "" {
		dist = os.DirFS(d)
	} else if sub, err := fs.Sub(assets, "frontend/dist"); err == nil {
		dist = sub
	} else {
		return err
	}
	files := http.FileServerFS(dist)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			files.ServeHTTP(w, r)
			return
		}
		b, _ := fs.ReadFile(dist, "index.html")
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, strings.Replace(string(b), "<head>", "<head>"+shim, 1))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		ch := make(chan []byte, 64)
		mu.Lock()
		subs[ch] = true
		mu.Unlock()
		defer func() { mu.Lock(); delete(subs, ch); mu.Unlock() }()
		fl := w.(http.Flusher)
		io.WriteString(w, ": ok\n\n")
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case b := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
		}
	})
	mux.HandleFunc("/rpc/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/rpc/")
		m := reflect.ValueOf(app).MethodByName(name)
		reply := func(v any) { w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode(v) }
		if !m.IsValid() {
			reply(map[string]any{"error": "no method " + name})
			return
		}
		var raw []json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		t := m.Type()
		in := make([]reflect.Value, t.NumIn())
		for i := range in {
			p := reflect.New(t.In(i))
			if i < len(raw) {
				if err := json.Unmarshal(raw[i], p.Interface()); err != nil {
					reply(map[string]any{"error": err.Error()})
					return
				}
			}
			in[i] = p.Elem()
		}
		out := m.Call(in)
		res := map[string]any{}
		for _, v := range out {
			if e, ok := v.Interface().(error); ok && e != nil {
				res["error"] = e.Error()
			} else if v.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
				continue
			} else {
				res["result"] = v.Interface()
			}
		}
		reply(res)
	})
	fmt.Println("rowan dev server on http://" + addr)
	return http.ListenAndServe(addr, mux)
}
