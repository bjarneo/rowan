package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"rowan/internal/config"
	"rowan/internal/db"
)

type cachedResult struct {
	name   string
	engine string
	res    *db.Result
}

// resultCache keeps the newest results in Go so that export does not send
// rows back from the UI.
type resultCache struct {
	mu    sync.Mutex
	max   int
	order []string
	items map[string]cachedResult
}

func newResultCache(max int) *resultCache {
	return &resultCache{max: max, items: map[string]cachedResult{}}
}

func (c *resultCache) put(name, engine string, r *db.Result) string {
	id := config.NewID("r")
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[id] = cachedResult{name: name, engine: engine, res: r}
	c.order = append(c.order, id)
	for len(c.order) > c.max {
		delete(c.items, c.order[0])
		c.order = c.order[1:]
	}
	return id
}

func (c *resultCache) get(id string) (cachedResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.items[id]
	return r, ok
}

// Export writes a cached result to a file that the user picks. It returns
// the path, or "" when the user canceled.
func (a *App) Export(resultID, format string) (string, error) {
	r, ok := a.results.get(resultID)
	if !ok {
		return "", errors.New("this result is no longer in memory. Run the query again")
	}
	ext := map[string]string{"csv": "csv", "json": "json", "sql": "sql"}[format]
	if ext == "" {
		return "", fmt.Errorf("unknown format %q", format)
	}
	c := a.ctx()
	if c == nil {
		return "", errors.New("the window is not ready yet. Try the export again")
	}
	path, err := wr.SaveFileDialog(c, wr.SaveDialogOptions{
		Title:           "Export " + strings.ToUpper(ext),
		DefaultFilename: r.name + "." + ext,
		Filters:         []wr.FileFilter{{DisplayName: strings.ToUpper(ext) + " files", Pattern: "*." + ext}},
	})
	if err != nil || path == "" {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	w := bufio.NewWriterSize(f, 256*1024)
	switch format {
	case "csv":
		err = writeCSV(w, r.res)
	case "json":
		err = writeJSON(w, r.res)
	case "sql":
		err = writeSQL(w, r.name, r.engine, r.res)
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	return config.DisplayPath(path), nil
}

func writeCSV(w *bufio.Writer, r *db.Result) error {
	cw := csv.NewWriter(w)
	head := make([]string, len(r.Columns))
	for i, c := range r.Columns {
		head[i] = c.Name
	}
	if err := cw.Write(head); err != nil {
		return err
	}
	rec := make([]string, len(r.Columns))
	for _, row := range r.Rows {
		for i, v := range row {
			if v == nil {
				rec[i] = ""
			} else {
				rec[i] = fmt.Sprint(v)
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeJSON(w *bufio.Writer, r *db.Result) error {
	w.WriteString("[\n")
	for i, row := range r.Rows {
		w.WriteString("  {")
		for j, c := range r.Columns {
			if j > 0 {
				w.WriteString(", ")
			}
			k, _ := json.Marshal(c.Name)
			w.Write(k)
			w.WriteString(": ")
			v := row[j]
			switch {
			case v == nil:
				w.WriteString("null")
			case c.Numeric:
				w.WriteString(fmt.Sprint(v))
			default:
				b, _ := json.Marshal(v)
				w.Write(b)
			}
		}
		w.WriteString("}")
		if i < len(r.Rows)-1 {
			w.WriteString(",")
		}
		w.WriteString("\n")
	}
	_, err := w.WriteString("]\n")
	return err
}

func writeSQL(w *bufio.Writer, table, engine string, r *db.Result) error {
	cols := make([]string, len(r.Columns))
	for i, c := range r.Columns {
		cols[i] = db.QuoteIdent(engine, c.Name)
	}
	head := "INSERT INTO " + db.QuoteIdent(engine, table) + " (" + strings.Join(cols, ", ") + ") VALUES ("
	for _, row := range r.Rows {
		w.WriteString(head)
		for j, v := range row {
			if j > 0 {
				w.WriteString(", ")
			}
			switch {
			case v == nil:
				w.WriteString("NULL")
			case r.Columns[j].Numeric:
				w.WriteString(fmt.Sprint(v))
			default:
				w.WriteString("'" + strings.ReplaceAll(fmt.Sprint(v), "'", "''") + "'")
			}
		}
		if _, err := w.WriteString(");\n"); err != nil {
			return err
		}
	}
	return nil
}
