// Package render turns self-contained HTML into a PDF. The only backend is a
// long-lived headless Chromium driven by chromedp (SPEC.md §7); the Renderer
// interface keeps it swappable.
package render

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Renderer produces a PDF from a complete HTML document.
type Renderer interface {
	PDF(ctx context.Context, html string) ([]byte, error)
}

// ErrUnavailable is returned when no Chromium binary could be found.
var ErrUnavailable = errors.New("render: no Chromium/Chrome binary found (set PP_CHROME)")

// Timeout for a single render.
const Timeout = 30 * time.Second

// candidates are tried in order when PP_CHROME is not set.
var candidates = []string{
	"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome",
	"/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	`C:\Program Files\Google\Chrome\Application\chrome.exe`,
	`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
}

// FindChrome returns the browser binary path ("" if none).
func FindChrome() string {
	if p := os.Getenv("PP_CHROME"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if p2, err := exec.LookPath(p); err == nil {
			return p2
		}
		return ""
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// blockAll denies every scheme that could leave the tab; data: URIs (the
// inlined fonts and assets) never hit the network layer.
var blockAll = []*network.BlockPattern{
	{URLPattern: "http://*:*/*", Block: true},
	{URLPattern: "https://*:*/*", Block: true},
	{URLPattern: "ws://*:*/*", Block: true},
	{URLPattern: "wss://*:*/*", Block: true},
	{URLPattern: "ftp://*:*/*", Block: true},
	{URLPattern: "file:///*", Block: true},
}

// Chrome is a lazily started, shared headless browser.
type Chrome struct {
	path string
	sem  chan struct{}

	mu          sync.Mutex
	browserCtx  context.Context
	cancelAlloc context.CancelFunc
	cancelBrows context.CancelFunc
}

// NewChrome prepares a renderer for the given binary path (see FindChrome).
// Nothing is started until the first PDF call. concurrency caps parallel tabs.
func NewChrome(path string, concurrency int) *Chrome {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Chrome{path: path, sem: make(chan struct{}, concurrency)}
}

// Available reports whether a browser binary is configured.
func (c *Chrome) Available() bool { return c != nil && c.path != "" }

// Path of the browser binary.
func (c *Chrome) Path() string { return c.path }

func (c *Chrome) start() (context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.browserCtx != nil && c.browserCtx.Err() == nil {
		return c.browserCtx, nil
	}
	c.stopLocked()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(c.path),
		chromedp.Flag("headless", "new"),
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	// Start the browser now so the first real render is fast and errors are
	// early. chromedp ties the process to the context of this first Run, so it
	// must be browserCtx itself; the timeout is enforced from the outside.
	errc := make(chan error, 1)
	go func() { errc <- chromedp.Run(browserCtx) }()
	select {
	case err := <-errc:
		if err != nil {
			cancelBrowser()
			cancelAlloc()
			return nil, fmt.Errorf("render: start browser: %w", err)
		}
	case <-time.After(30 * time.Second):
		cancelBrowser()
		cancelAlloc()
		return nil, fmt.Errorf("render: start browser: timeout")
	}
	c.browserCtx, c.cancelAlloc, c.cancelBrows = browserCtx, cancelAlloc, cancelBrowser
	return browserCtx, nil
}

func (c *Chrome) stopLocked() {
	if c.cancelBrows != nil {
		c.cancelBrows()
	}
	if c.cancelAlloc != nil {
		c.cancelAlloc()
	}
	c.browserCtx, c.cancelAlloc, c.cancelBrows = nil, nil, nil
}

// Close shuts the browser down (safe to call when never started).
func (c *Chrome) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

// PDF renders html in a fresh, network-isolated, script-disabled tab.
func (c *Chrome) PDF(ctx context.Context, html string) ([]byte, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	pdf, err := c.renderOnce(ctx, html)
	if err != nil && ctx.Err() == nil {
		// The browser may have died (OOM, crash): restart once and retry.
		c.mu.Lock()
		c.stopLocked()
		c.mu.Unlock()
		pdf, err = c.renderOnce(ctx, html)
	}
	return pdf, err
}

func (c *Chrome) renderOnce(ctx context.Context, html string) ([]byte, error) {
	browserCtx, err := c.start()
	if err != nil {
		return nil, err
	}
	tabCtx, cancelTab := chromedp.NewContext(browserCtx)
	defer cancelTab()
	tctx, cancel := context.WithTimeout(tabCtx, Timeout)
	defer cancel()
	// Propagate the caller's cancellation into the tab.
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	var pdf []byte
	err = chromedp.Run(tctx,
		emulation.SetScriptExecutionDisabled(true),
		network.Enable(),
		network.SetBlockedURLs().WithURLPatterns(blockAll),
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(tree.Frame.ID, html).Do(ctx)
		}),
		// Wait for the inlined fonts to be ready (DevTools evaluation still
		// works with page scripts disabled).
		chromedp.ActionFunc(func(ctx context.Context) error {
			var status string
			return chromedp.Evaluate(`document.fonts.ready.then(() => document.fonts.status)`, &status,
				func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPreferCSSPageSize(true).
				WithDisplayHeaderFooter(false).
				Do(ctx)
			pdf = buf
			return err
		}),
	)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("render: %w", err)
	}
	return pdf, nil
}
