//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"
	"time"
)

func main() {
	meta := js.Global().Get("document").Call("querySelector", `meta[name="app-version"]`)
	loadedVersion := ""
	if !meta.IsNull() && !meta.IsUndefined() {
		loadedVersion = meta.Call("getAttribute", "content").String()
	}

	check := js.FuncOf(func(this js.Value, args []js.Value) any {
		go checkVersion(loadedVersion)
		return nil
	})

	brewhouse := js.Global().Get("Object").New()
	brewhouse.Set("onNavigate", check)
	brewhouse.Set("checkVersion", check)
	js.Global().Set("Brewhouse", brewhouse)

	// Periodic version check every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			checkVersion(loadedVersion)
		}
	}()

	select {}
}

func checkVersion(loaded string) {
	resp, err := jsFetch("/api/version")
	if err != nil || resp == "" {
		return
	}
	var payload struct {
		Version string `json:"version"`
	}
	if json.Unmarshal([]byte(resp), &payload) != nil {
		return
	}
	if payload.Version != "" && loaded != "" && payload.Version != loaded {
		js.Global().Get("location").Call("reload")
	}
}

func jsFetch(url string) (string, error) {
	done := make(chan struct {
		body string
		err  error
	}, 1)

	promise := js.Global().Call("fetch", url)
	promise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
		resp := args[0]
		resp.Call("text").Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
			done <- struct {
				body string
				err  error
			}{body: args[0].String()}
			return nil
		}))
		return nil
	})).Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- struct {
			body string
			err  error
		}{err: js.Error{Value: args[0]}}
		return nil
	}))

	result := <-done
	return result.body, result.err
}
