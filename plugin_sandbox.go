package main

import (
	"fmt"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// PluginHookResult holds the output of a plugin hook execution.
type PluginHookResult struct {
	Modified bool   // did the plugin modify the content?
	Content  string // modified content (or original if not modified)
	Error    error
}

// newSandbox creates a restricted Lua state with safe stdlib only.
// No os, no io, no package, no debug — string/math/table only.
func newSandbox() *lua.LState {
	L := lua.NewState(lua.Options{
		SkipOpenLibs: true,
	})
	// Open only safe libs
	lua.OpenString(L)
	lua.OpenMath(L)
	lua.OpenTable(L)
	lua.OpenBase(L) // print, tostring, type, ipairs, pairs, etc.
	return L
}

// RunBeforeResponseGenerate runs the plugin's beforeResponseGenerate(prompt) hook.
// Called before the user message is sent to the LLM.
// The hook may modify the prompt — return modified version or original.
func RunBeforeResponseGenerate(pluginCode, prompt string) PluginHookResult {
	if strings.TrimSpace(pluginCode) == "" {
		return PluginHookResult{Content: prompt}
	}

	L := newSandbox()
	defer L.Close()

	// 30ms timeout — plugins must be fast
	done := make(chan PluginHookResult, 1)
	go func() {
		if err := L.DoString(pluginCode); err != nil {
			done <- PluginHookResult{Content: prompt, Error: fmt.Errorf("plugin load error: %w", err)}
			return
		}

		fn := L.GetGlobal("beforeResponseGenerate")
		if fn == lua.LNil {
			done <- PluginHookResult{Content: prompt}
			return
		}

		err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    1,
			Protect: true,
		}, lua.LString(prompt))
		if err != nil {
			done <- PluginHookResult{Content: prompt, Error: fmt.Errorf("beforeResponseGenerate error: %w", err)}
			return
		}

		result := L.Get(-1)
		L.Pop(1)
		if result == lua.LNil || result.Type() != lua.LTString {
			done <- PluginHookResult{Content: prompt}
			return
		}
		modified := result.String()
		done <- PluginHookResult{Modified: modified != prompt, Content: modified}
	}()

	select {
	case res := <-done:
		return res
	case <-time.After(30 * time.Millisecond):
		return PluginHookResult{Content: prompt, Error: fmt.Errorf("plugin timeout (>30ms)")}
	}
}

// RunAfterResponseGenerate runs the plugin's afterResponseGenerate(response) hook.
// Called after the LLM responds, before the message is shown to the user.
// The hook may modify the response — return modified version or original.
func RunAfterResponseGenerate(pluginCode, response string) PluginHookResult {
	if strings.TrimSpace(pluginCode) == "" {
		return PluginHookResult{Content: response}
	}

	L := newSandbox()
	defer L.Close()

	done := make(chan PluginHookResult, 1)
	go func() {
		if err := L.DoString(pluginCode); err != nil {
			done <- PluginHookResult{Content: response, Error: fmt.Errorf("plugin load error: %w", err)}
			return
		}

		fn := L.GetGlobal("afterResponseGenerate")
		if fn == lua.LNil {
			done <- PluginHookResult{Content: response}
			return
		}

		err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    1,
			Protect: true,
		}, lua.LString(response))
		if err != nil {
			done <- PluginHookResult{Content: response, Error: fmt.Errorf("afterResponseGenerate error: %w", err)}
			return
		}

		result := L.Get(-1)
		L.Pop(1)
		if result == lua.LNil || result.Type() != lua.LTString {
			done <- PluginHookResult{Content: response}
			return
		}
		modified := result.String()
		done <- PluginHookResult{Modified: modified != response, Content: modified}
	}()

	select {
	case res := <-done:
		return res
	case <-time.After(30 * time.Millisecond):
		return PluginHookResult{Content: response, Error: fmt.Errorf("plugin timeout (>30ms)")}
	}
}
