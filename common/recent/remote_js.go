// The recent-files list in the browser: the WebAssembly build has no
// config directory (a list in the page's memory was empty in every new
// tab and lost on reload), so it keeps the list on the web server, per
// user (GET and PUT /api/recent, web/server/files.go). It reloads when the
// page comes back into view, to show what other tabs opened meanwhile.

//go:build js

package recent

import (
	"encoding/json"
	"errors"
	"fmt"
	"syscall/js"
)

// recentURL is the web server's address for the list.
const recentURL = "/api/recent"

// webRemote is the list on the web server.
type webRemote struct{}

// defaultRemote is the web server's list when the page is OfficeLite's
// web client (its serverfs.js defines officeweb), else nil.
func defaultRemote() remote {
	g := js.Global()
	if !g.Get("officeweb").Truthy() {
		return nil
	}
	refresh := js.FuncOf(func(js.Value, []js.Value) any {
		if g.Get("document").Get("visibilityState").String() == "visible" && defaultList != nil {
			defaultList.Refresh()
		}
		return nil
	})
	g.Get("document").Call("addEventListener", "visibilitychange", refresh)
	return webRemote{}
}

// load fetches the list.
func (webRemote) load() ([]Entry, error) {
	body, err := request("GET", nil)
	if err != nil {
		return nil, err
	}
	var list []Entry
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return nil, err
	}
	return list, nil
}

// save stores list.
func (webRemote) save(list []Entry) error {
	if list == nil {
		list = []Entry{}
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = request("PUT", data)
	return err
}

// request sends method to recentURL with body (when not nil) and returns
// the answer's text, or an error for a failed request or status.
func request(method string, body []byte) (string, error) {
	opts := map[string]any{"method": method, "credentials": "same-origin", "cache": "no-store"}
	if body != nil {
		opts["body"] = string(body)
		opts["headers"] = map[string]any{"Content-Type": "application/json"}
	}
	resp, err := await(js.Global().Call("fetch", recentURL, opts))
	if err != nil {
		return "", err
	}
	text, err := await(resp.Call("text"))
	if err != nil {
		return "", err
	}
	if !resp.Get("ok").Bool() {
		return "", fmt.Errorf("%s %s: %d %s", method, recentURL, resp.Get("status").Int(), text.String())
	}
	return text.String(), nil
}

// await waits for promise p to settle. It must not run on a goroutine
// called from JavaScript (an event handler), which would block the page.
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{v: args[0]}
		return nil
	})
	defer then.Release()
	catch := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{err: errors.New(args[0].Call("toString").String())}
		return nil
	})
	defer catch.Release()
	p.Call("then", then, catch)
	r := <-ch
	return r.v, r.err
}
